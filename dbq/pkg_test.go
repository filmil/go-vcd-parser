package dbq

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/dbt"
	"github.com/filmil/go-vcd-parser/vcd"
)

func TestBasic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dbx, err := db.OpenDB(ctx, dbt.NewMemDB())
	if err != nil {
		t.Fatalf("could not open DB: %v", err)
	}
	i := dbt.New(dbx, ctx)
	// Example of multiple signal addition.
	i.
		Signal("//clk", vcd.VarKindLogic, 1).
		//
		// //clk   ________/~~~~~~~~~~...
		//         ^0      ^100
		TimeValues([]dbt.TimeValue{{0, "0"}, {100, "Z"}}...)

	// Create a query engine.
	q := New(dbx)

	// Demo query: just find first value of the signal.
	s := q.Signal("//clk")
	ts := s.FindFirst("Z")

	// Crude examination.
	if ts.Error() != nil {
		t.Fatalf("in timestamp: %v", ts.Error())
	}
	if ts.IsNone() || ts.T() != 100 {
		t.Errorf("mismatch: %+v:", ts)
	}

}

func TestValueAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dbx, err := db.OpenDB(ctx, dbt.NewMemDB())
	if err != nil {
		t.Fatalf("could not open DB: %v", err)
	}
	i := dbt.New(dbx, ctx)
	// Example of multiple signal addition.
	i.
		Signal("//clk", vcd.VarKindLogic, 1).
		//
		// //clk   ________/~~~~~~~~~~...
		//         ^0      ^100
		TimeValues([]dbt.TimeValue{{0, "0"}, {100, "Z"}, {200, "1"}}...)

	// Create a query engine.
	q := New(dbx)

	// Demo query: just find first value of the signal.
	s := q.Signal("//clk")
	ts := s.FindFirst("1")
	v := s.ValueAt(ts)
	if v.Error() != nil {
		t.Fatalf("in timestamp: %v", ts.Error())
	}
	if v.IsNone() || v.V() != "Z" {
		t.Errorf("mismatch: %v:", spew.Sdump(v))
	}
	p := s.PrevChange(ts)
	if p.ValueAt() != "Z" {
		t.Errorf("mismatch: %v", spew.Sdump(p))
	}

	ts = s.FindFirst("0")
	n := s.NextChange(ts)
	if n.ValueAt() != "Z" {
		t.Errorf("mismatch: %v", spew.Sdump(n))
	}
	nn := s.NextChange(s.NextChange(ts))
	if nn.ValueAt() != "1" {
		t.Errorf("mismatch: %v", spew.Sdump(nn))
	}
}
