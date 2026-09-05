package db

import (
	"context"
	"path/filepath"
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

// TestWideValueSurvives holds the columns to TEXT. A column whose
// declared type is neither TEXT nor INT has numeric affinity in
// SQLite, so a STRING column turned `00000001` into the integer 1 and
// a 22 bit value into the double 1.11111111111111e+21, which is the
// value destroyed: every VCD signal wider than about 18 bits came back
// as a number.
func TestWideValueSurvives(t *testing.T) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	// A file rather than DefaultFilename: the in-memory database is
	// shared by name, so a second test that opened it would find the
	// schema already there.
	name := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(ctx, name)
	if err != nil {
		t.Fatalf("could not open: %v: %v", name, err)
	}
	values := []string{"00000001", "1111111111111111111111", "101", "xxxx"}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("could not create tx: %v", err)
	}
	if err := AddSignal(ctx, tx, "wide", vcd.VarKindWire, "&!", 22); err != nil {
		t.Fatalf("could not add signal: %v", err)
	}
	for i, v := range values {
		if err := AddValue(ctx, tx, uint64(i), "&!", v); err != nil {
			t.Fatalf("could not add value: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("could not commit: %v", err)
	}
	rows, err := db.Query(`SELECT Value FROM Svalues WHERE Code = '&!' ORDER BY Id;`)
	if err != nil {
		t.Fatalf("could not query: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("could not scan: %v", err)
		}
		got = append(got, v)
	}
	if len(got) != len(values) {
		t.Fatalf("read %d values, wrote %d", len(got), len(values))
	}
	for i, v := range values {
		if got[i] != v {
			t.Errorf("value %d came back as %q, wrote %q", i, got[i], v)
		}
	}
}

// TestMeta round trips the table that says which unit the timestamps
// count.
func TestMeta(t *testing.T) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	name := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(ctx, name)
	if err != nil {
		t.Fatalf("could not open: %v: %v", name, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("could not create tx: %v", err)
	}
	if err := SetMeta(ctx, tx, "timescale", "1ps"); err != nil {
		t.Fatalf("could not set: %v", err)
	}
	v, ok, err := GetMeta(ctx, tx, "timescale")
	if err != nil {
		t.Fatalf("could not get: %v", err)
	}
	if !ok || v != "1ps" {
		t.Errorf("timescale is %q, %v; want 1ps, true", v, ok)
	}
	if _, ok, err := GetMeta(ctx, tx, "absent"); err != nil || ok {
		t.Errorf("absent key is %v, %v; want false, nil", ok, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("could not commit: %v", err)
	}
}
