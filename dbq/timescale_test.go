package dbq

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/dbt"
	"github.com/filmil/go-vcd-parser/vcd"
)

// newSignalDB builds a one-signal database whose transitions are at the
// given raw timestamps. timescale, when not empty, is recorded in Meta the
// way a conversion records it.
func newSignalDB(t *testing.T, timescale string, ticks ...uint64) *sql.DB {
	t.Helper()
	ctx := context.Background()
	dbx, err := db.OpenDB(ctx, dbt.NewMemDB())
	if err != nil {
		t.Fatalf("could not open DB: %v", err)
	}
	if timescale != "" {
		tx, err := dbx.Begin()
		if err != nil {
			t.Fatalf("could not begin: %v", err)
		}
		if err := db.SetMeta(ctx, tx, "timescale_seconds", timescale); err != nil {
			t.Fatalf("could not set meta: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("could not commit: %v", err)
		}
	}
	var pairs []dbt.TimeValue
	for i, tick := range ticks {
		pairs = append(pairs, dbt.TimeValue{Time: tick, Value: []string{"0", "1"}[i%2]})
	}
	dbt.New(dbx, ctx).Signal("//clk", vcd.VarKindLogic, 1).TimeValues(pairs...)
	return dbx
}

// TestTimescaleIsReadFromMeta is the fix for timestamps being reported in
// the wrong unit. The same raw tick count means a different duration
// depending on what the file's $timescale said, and the answer is in Meta.
func TestTimescaleIsReadFromMeta(t *testing.T) {
	tests := []struct {
		name      string
		timescale string
		wantD     time.Duration
		wantFs    int64
	}{
		// A femtosecond dump, as nvc writes: 1e6 ticks is 1 ns.
		{"femtoseconds", "1e-15", 1 * time.Nanosecond, 1_000_000},
		// A picosecond dump: 1e6 ticks is 1 us.
		{"picoseconds", "1e-12", 1 * time.Microsecond, 1_000_000_000},
		// A nanosecond dump: 1e6 ticks is 1 ms.
		{"nanoseconds", "1e-09", 1 * time.Millisecond, 1_000_000_000_000},
		// No Meta at all, as a database written before it existed: the
		// historical picosecond assumption is kept.
		{"unrecorded", "", 1 * time.Microsecond, 1_000_000_000},
		// A timescale that is not a number falls back the same way.
		{"unparseable", "banana", 1 * time.Microsecond, 1_000_000_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbx := newSignalDB(t, test.timescale, 0, 1_000_000)
			ts := New(dbx).Signal("//clk").FindFirst("1")
			if err := ts.IsOk(); err != nil {
				t.Fatalf("no transition: %v", err)
			}
			if got := ts.T(); got != 1_000_000 {
				t.Fatalf("raw tick is %v, want 1000000", got)
			}
			if got := ts.D(); got != test.wantD {
				t.Errorf("D() is %v, want %v", got, test.wantD)
			}
			if got := ts.Fs(); got != test.wantFs {
				t.Errorf("Fs() is %v, want %v", got, test.wantFs)
			}
		})
	}
}

// TestDurationsAreRelativeToTheTimescale covers the assertion helpers,
// which are built on D() and so were wrong by the same factor.
func TestDurationsAreRelativeToTheTimescale(t *testing.T) {
	// A 200 MHz clock in a 1fs dump: rising edges 5 ns apart.
	dbx := newSignalDB(t, "1e-15", 0, 1_000_000, 3_500_000, 6_000_000)
	q := New(dbx)
	clk := q.Signal("//clk")

	first := clk.FindFirst("1")
	if err := first.IsOk(); err != nil {
		t.Fatalf("no rising edge: %v", err)
	}
	next := clk.FindAfter(first, "1")
	if err := next.IsOk(); err != nil {
		t.Fatalf("no second rising edge: %v", err)
	}
	if got, want := Diff(first, next), 5*time.Nanosecond; got != want {
		t.Errorf("period is %v, want %v", got, want)
	}
	if err := IsDurationApprox(first, next, 5*time.Nanosecond); err != nil {
		t.Errorf("IsDurationApprox: %v", err)
	}
}

// TestSubNanosecondNeedsFs documents why Fs exists: a duration counts
// whole nanoseconds, so anything finer disappears from D().
func TestSubNanosecondNeedsFs(t *testing.T) {
	// One tick of a 1fs dump is 1 fs, far below a nanosecond.
	dbx := newSignalDB(t, "1e-15", 0, 500_000)
	ts := New(dbx).Signal("//clk").FindFirst("1")
	if err := ts.IsOk(); err != nil {
		t.Fatalf("no transition: %v", err)
	}
	if got := ts.Fs(); got != 500_000 {
		t.Errorf("Fs() is %v, want 500000", got)
	}
	if got := ts.D(); got != 0 {
		t.Errorf("D() is %v, want 0: half a nanosecond does not fit", got)
	}
}
