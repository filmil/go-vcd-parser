package cvt

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

// Sink writes a VCD file to a signals database as it is read. It is the
// single conversion path: ConvertStream drives it from the streaming
// parser, and Convert drives it from an already-parsed File.
//
// It holds one transaction at a time, committing every MaxTx operations.
type Sink struct {
	ctx context.Context
	dbf *sql.DB
	tx  *sql.Tx
	ins *db.Inserter

	scope     []string
	timestamp uint64
	count     int
}

// NewSink opens the first transaction on dbf.
func NewSink(ctx context.Context, dbf *sql.DB) (*Sink, error) {
	s := &Sink{ctx: ctx, dbf: dbf, scope: []string{"/"}}
	if err := s.begin(); err != nil {
		return nil, fmt.Errorf("cvt.NewSink: %w", err)
	}
	if err := db.SetMeta(ctx, s.tx, "generator", "go-vcd-parser"); err != nil {
		return nil, fmt.Errorf("cvt.NewSink: %w", err)
	}
	return s, nil
}

// begin opens a transaction and prepares the insert statements on it.
func (s *Sink) begin() error {
	tx, err := s.dbf.Begin()
	if err != nil {
		return fmt.Errorf("could not begin: %w", err)
	}
	ins, err := db.Prepare(s.ctx, tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	s.tx, s.ins = tx, ins
	return nil
}

// commit closes the statements and commits the transaction.
func (s *Sink) commit() error {
	if err := s.ins.Close(); err != nil {
		return fmt.Errorf("could not close statements: %w", err)
	}
	if err := s.tx.Commit(); err != nil {
		return fmt.Errorf("could not commit: %w", err)
	}
	s.tx, s.ins = nil, nil
	return nil
}

// step counts one written row and rolls the transaction over when it grows
// past MaxTx.
func (s *Sink) step() error {
	s.count++
	if s.count%MaxTx != 0 {
		return nil
	}
	if err := s.commit(); err != nil {
		return fmt.Errorf("cvt: %w", err)
	}
	if err := s.begin(); err != nil {
		return fmt.Errorf("cvt: %w", err)
	}
	return nil
}

// Close commits the final transaction.
func (s *Sink) Close() error {
	if s.tx == nil {
		return nil
	}
	if err := s.commit(); err != nil {
		return fmt.Errorf("cvt: %w", err)
	}
	return nil
}

// Declaration tracks the scope stack and writes each $var as a signal.
func (s *Sink) Declaration(e *vcd.DeclarationCommandT) error {
	switch {
	case e.Scope != nil:
		s.scope = append(s.scope, e.Scope.Id)
	case e.Upscope != nil:
		if len(s.scope) < 2 {
			return nil
		}
		s.scope = s.scope[:len(s.scope)-1]
	case e.Timescale != nil:
		// The timestamps of Svalues count this unit, and neither
		// Signals nor Svalues has anywhere to say which unit that is.
		if err := db.SetMeta(s.ctx, s.tx, "timescale", timescaleText(e.Timescale)); err != nil {
			return fmt.Errorf("cvt.Sink: %w", err)
		}
		sec := strconv.FormatFloat(e.Timescale.AsSeconds(), 'g', -1, 64)
		if err := db.SetMeta(s.ctx, s.tx, "timescale_seconds", sec); err != nil {
			return fmt.Errorf("cvt.Sink: %w", err)
		}
	case e.Var != nil:
		v := e.Var
		name := strings.Join(append(s.scope, v.Id.String()), "/")
		if err := s.ins.AddSignal(s.ctx, name, v.GetVarKind(), v.Code, v.Size); err != nil {
			return fmt.Errorf("cvt.Sink: %w", err)
		}
		return s.step()
	}
	return nil
}

func (s *Sink) Timestamp(ts uint64, _ []byte) error {
	glog.V(3).Infof("cvt.Sink: timestamp: %v", ts)
	s.timestamp = ts
	return nil
}

func (s *Sink) ValueChange(kind vcd.ValueKind, value, idcode []byte) error {
	return s.addValue(string(idcode), string(kind.Payload(value)))
}

func (s *Sink) addValue(code, value string) error {
	// Note the explicit guard: glog.V(4).Infof would evaluate its
	// arguments -- including a full spew dump -- on every value change,
	// at every verbosity.
	if glog.V(4) {
		glog.Infof("cvt.Sink.addValue: %v, %v", code, value)
	}
	if err := s.ins.AddValue(s.ctx, s.timestamp, code, value); err != nil {
		return fmt.Errorf("cvt.Sink: could not add value: %w", err)
	}
	return s.step()
}

func (s *Sink) DumpBegin(vcd.DumpKind) error { return nil }
func (s *Sink) DumpEnd(vcd.DumpKind) error   { return nil }

func (s *Sink) Directive(string, []byte) error { return nil }

// ConvertStream reads a VCD file from r straight into an empty database,
// without ever holding the file, its tokens, or its parse tree in memory.
func ConvertStream(ctx context.Context, filename string, r io.Reader, dbf *sql.DB) error {
	s, err := NewSink(ctx, dbf)
	if err != nil {
		return err
	}
	if err := vcd.Parse(filename, r, s); err != nil {
		return fmt.Errorf("cvt.ConvertStream: %w", err)
	}
	return s.Close()
}

// replayChanges feeds the value changes of a dump block to the sink.
func (s *Sink) replayChanges(vcs []*vcd.ValueChangeT) error {
	for _, v := range vcs {
		if glog.V(4) {
			glog.Infof("cvt: value change: %v", spew.Sdump(*v))
		}
		if err := s.addValue(v.GetIdCode(), v.GetValue()); err != nil {
			return err
		}
	}
	return nil
}

// timescaleText spells a $timescale the way the file writes it, `1ps`.
func timescaleText(t *vcd.TimescaleT) string {
	return fmt.Sprintf("%d%s", t.Number, unitName(t.Unit))
}

// unitName is the suffix of a time unit, empty for a unit the parser
// did not fill in.
func unitName(u *vcd.TimeUnit) string {
	if u == nil {
		return ""
	}
	switch {
	case u.Second:
		return "s"
	case u.MilliSecond:
		return "ms"
	case u.MicroSecond:
		return "us"
	case u.NanoSecond:
		return "ns"
	case u.PicoSecond:
		return "ps"
	case u.FemtoSecond:
		return "fs"
	}
	return ""
}
