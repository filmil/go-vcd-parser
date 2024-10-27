package db

import (
	"context"
	"testing"

	"github.com/filmil/go-vcd-parser/vcd"
)

func TestInsertRead(t *testing.T) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	db, err := OpenDB(ctx, DefaultFilename)
	if err != nil {
		t.Fatalf("could not open: %v: %v", DefaultFilename, err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("could not create tx: %v", err)
	}

	AddSignal(ctx, tx, "signal", vcd.VarKindReg, "^!", 1)

	if err := tx.Commit(); err != nil {
		t.Fatalf("could not commit: %v", err)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatalf("could not create tx: %v", err)
	}

	res := FindSignalByName(ctx, tx, "signal")
	var (
		kind vcd.VarKindCode
		code string
		size int
	)
	err = res.Scan(&kind, &code, &size)
	if err != nil {
		t.Fatalf("no scan: %v", err)
	}
	if kind.Int() == 0 {
		t.Errorf("kind is: %v", kind)
	}
	tx.Commit()

	// Let's insert some signals

	tx, err = db.Begin()
	AddValue(ctx, tx, 1, "^!", "1")
	AddValue(ctx, tx, 2, "^!", "0")
	if err := tx.Commit(); err != nil {
		t.Fatalf("could not commit: %v", err)
	}
}
