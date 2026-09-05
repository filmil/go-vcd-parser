// Package cvt converts a parsed VCD file to a database.
package cvt

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

// TxFactory opens a new transaction.
//
// Deprecated: the conversion holds a single transaction at a time; see Sink.
type TxFactory func() (*sql.Tx, error)

func InsertSignal(ctx context.Context, tx *sql.Tx,
	name string, kind vcd.VarKindCode, code string, size int) error {
	if err := db.AddSignal(ctx, tx, name, kind, code, size); err != nil {
		return fmt.Errorf("cvt.InsertSignal: error in tx: %w", err)
	}
	return nil
}

func InsertValueChange(ctx context.Context, tx *sql.Tx, ts uint64, vc *vcd.ValueChangeT) error {
	if glog.V(4) {
		glog.Infof("cvt.InsertValueChange: %v, %v", vc.GetIdCode(), vc.GetValue())
	}
	if err := db.AddValue(ctx, tx, ts, vc.GetIdCode(), vc.GetValue()); err != nil {
		return fmt.Errorf("cvt.InsertValueChange: could not add value: %w", err)
	}
	return nil
}

// MaxTx is the maximum number of operations in a transaction.
var MaxTx int = 100000

// InsertValueChanges writes a batch of value changes, all at timestamp.
//
// Deprecated: use Sink, which shares one transaction with the rest of the
// conversion instead of opening a second one alongside it.
func InsertValueChanges(ctx context.Context, txf TxFactory, timestamp uint64, vc []*vcd.ValueChangeT) error {
	tx, err := txf()
	if err != nil {
		return fmt.Errorf("cvt.InsertValueChanges: %w", err)
	}
	for i, v := range vc {
		if i != 0 && i%MaxTx == 0 {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("cvt.InsertValueChanges: error in commit: %w", err)
			}
			if tx, err = txf(); err != nil {
				return fmt.Errorf("cvt.InsertValueChanges: could not recreate tx: %w", err)
			}
		}
		if err := InsertValueChange(ctx, tx, timestamp, v); err != nil {
			return fmt.Errorf("cvt.InsertValueChanges: could not insert vc tx: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cvt.InsertValueChanges: error in commit: %w", err)
	}
	return nil
}

// Convert translates an already-parsed VCD file into an empty database.
//
// It replays the parse tree through the same Sink that ConvertStream
// drives, so both paths write identical rows. Prefer ConvertStream: it
// does not need the tree in memory.
func Convert(ctx context.Context, vcdFile *vcd.File, dbf *sql.DB) error {
	s, err := NewSink(ctx, dbf)
	if err != nil {
		return err
	}
	for _, e := range vcdFile.DeclarationCommand {
		if err := s.Declaration(e); err != nil {
			return fmt.Errorf("cvt.Convert: %w", err)
		}
	}
	for _, e := range vcdFile.SimulationCommand {
		var err error
		switch {
		case e.SimulationTime != nil:
			err = s.Timestamp(e.SimulationTime.Value(), nil)
		case e.Dumpvars != nil:
			err = s.replayChanges(e.Dumpvars.ValueChange)
		case e.Dumpall != nil:
			err = s.replayChanges(e.Dumpall.ValueChange)
		case e.Dumpon != nil:
			err = s.replayChanges(e.Dumpon.ValueChange)
		case e.Dumpoff != nil:
			err = s.replayChanges(e.Dumpoff.ValueChange)
		case e.ValueChange != nil:
			err = s.replayChanges([]*vcd.ValueChangeT{e.ValueChange})
		default:
			glog.V(3).Infof("unprocessed: %+v", e)
		}
		if err != nil {
			return fmt.Errorf("cvt.Convert: %w", err)
		}
	}
	return s.Close()
}
