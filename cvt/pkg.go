// Package cvt converts a parsed VCD file to a database.
package cvt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

type TxFactory func() (*sql.Tx, error)

func InsertSignal(ctx context.Context, tx *sql.Tx,
	name string, kind vcd.VarKindCode, code string, size int) error {
	if err := db.AddSignal(ctx, tx, name, kind, code, size); err != nil {
		return fmt.Errorf("cvt.InsertSignal: error in tx: %w", err)
	}
	return nil
}

func InsertValueChange(ctx context.Context, tx *sql.Tx, ts uint64, vc *vcd.ValueChangeT) error {
	glog.V(4).Infof("cvt.InsertValueChange: %v, %v, %v",
		vc.GetIdCode(), vc.GetValue(), spew.Sdump(*vc))
	if err := db.AddValue(ctx, tx, ts, vc.GetIdCode(), vc.GetValue()); err != nil {
		return fmt.Errorf("cvt.InsertValueChange: could not add value: %w", err)
	}
	return nil
}

// MaxTx is the maximum number of operations in a transaction.
var MaxTx int = 100000

func InsertValueChanges(ctx context.Context, txf TxFactory, timestamp uint64, vc []*vcd.ValueChangeT) error {
	tx, err := txf()
	if err != nil {
		return fmt.Errorf("cvt.InsertValueChanges: %w", err)
	}
	for i, v := range vc {
		if i != 0 && i%MaxTx == 0 {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("cvt.InsertValueChanges: error in commit: %v", err)
			}
			tx, err = txf()
			if err != nil {
				return fmt.Errorf("cvt.InsertValueChanges: could not recreate tx: %w", err)
			}
		}
		if err := InsertValueChange(ctx, tx, timestamp, v); err != nil {
			return fmt.Errorf("cvt.InsertValueChanges: could not insert vc tx: %w", err)
		}
	}
	defer func() {
		if err := tx.Commit(); err != nil {
			glog.Errorf("cvt.InsertValueChanges: error in commit: %v", err)
		}
	}()
	return nil
}

// Convert translates a parsed VCD file into an empty database.
func Convert(ctx context.Context, vcdFile *vcd.File, dbf *sql.DB) error {
	scope := []string{"/"}

	var txf TxFactory = func() (*sql.Tx, error) {
		return dbf.Begin()
	}

	var count int
	tx, err := txf()
	if err != nil {
		return fmt.Errorf("cvt.Convert: could not create a value change tx")
	}
	for _, e := range vcdFile.DeclarationCommand {
		switch {
		case e.EndDefinitions != nil:
			glog.V(2).Infof("cvt.Convert: enddefinitions found")
			break
		case e.Scope != nil:
			scope = append(scope, e.Scope.Id)
		case e.Upscope != nil:
			if len(scope) < 2 {
				continue
			}
			scope = scope[0 : len(scope)-2]
		case e.Var != nil:
			count++
			v := e.Var
			name := strings.Join(append(scope, v.Id.String()), "/")
			if err := InsertSignal(ctx, tx, name, v.GetVarKind(), v.Code, v.Size); err != nil {
				return fmt.Errorf("cvt.Convert: %w", err)
			}
			if count != 0 && count%MaxTx == 0 {
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("cvt.Convert: could not add value change: %w", err)
				}
				tx, err = txf()
				if err != nil {
					return fmt.Errorf("cvt.Convert: could not create a value change tx")
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cvt.Convert: could not add value change: %w", err)
	}

	var timestamp uint64
	count = 0
	tx, err = txf()
	if err != nil {
		return fmt.Errorf("cvt.Convert: could not create a value change tx")
	}
	for _, e := range vcdFile.SimulationCommand {
		switch {
		case e.SimulationTime != nil:
			s := e.SimulationTime
			timestamp = s.Value()
			glog.V(3).Infof("cvt.Convert: add timestamp: %v", timestamp)
		case e.Dumpvars != nil:
			d := e.Dumpvars
			err := InsertValueChanges(ctx, txf, timestamp, d.ValueChange)
			if err != nil {
				return fmt.Errorf("cvt.Convert: dumpvars %w", err)
			}
		case e.Dumpall != nil:
			d := e.Dumpall
			err := InsertValueChanges(ctx, txf, timestamp, d.ValueChange)
			if err != nil {
				return fmt.Errorf("cvt.Convert: dumpall %w", err)
			}
		case e.Dumpon != nil:
			d := e.Dumpon
			err := InsertValueChanges(ctx, txf, timestamp, d.ValueChange)
			if err != nil {
				return fmt.Errorf("cvt.Convert: dumpon %w", err)
			}
		case e.Dumpoff != nil:
			d := e.Dumpoff
			err := InsertValueChanges(ctx, txf, timestamp, d.ValueChange)
			if err != nil {
				return fmt.Errorf("cvt.Convert: dumpoff %w", err)
			}
		case e.ValueChange != nil:
			count++
			v := e.ValueChange
			err := InsertValueChange(ctx, tx, timestamp, v)
			if err != nil {
				return fmt.Errorf("cvt.Convert: could not add value change: %w", err)
			}
			if count != 0 && count%MaxTx == 0 {
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("cvt.Convert: could not add value change: %w", err)
				}
				tx, err = txf()
				if err != nil {
					return fmt.Errorf("cvt.Convert: could not create a value change tx")
				}
			}
		default:
			glog.V(3).Infof("unprocessed: %+v", e)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cvt.Convert: could not add value change: %w", err)
	}
	return nil
}
