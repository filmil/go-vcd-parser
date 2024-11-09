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
	ve := s.ValueAtP(ts)
	if ve.Error() != nil {
		t.Fatalf("in timestamp: %v", ts.Error())
	}
	if ve.IsNone() || ve.V() != "1" {
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

func TestFindFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dbx, err := db.OpenDB(ctx, dbt.NewMemDB())
	if err != nil {
		t.Fatalf("could not open DB: %v", err)
	}
	i := dbt.New(dbx, ctx)

	s1i := i.Signal("//clk1", vcd.VarKindLogic, 1)
	s2i := i.Signal("//clk2", vcd.VarKindLogic, 1)
	s3i := i.Signal("//clk3", vcd.VarKindLogic, 1)

	// //clk1 XXXX1111XXXX1111XXXX1111
	// //clk2 XXXX1111XXXX2222XXXX2222
	// //clk3 XXXX3333XXXX2222XXXX3333
	//        ^0  ^100^200^300^400^500
	s1i.TimeValues([]dbt.TimeValue{{0, "X"}, {100, "1"}, {200, "X"}, {300, "1"}, {400, "X"}, {500, "1"}}...)
	s2i.TimeValues([]dbt.TimeValue{{0, "X"}, {100, "1"}, {200, "X"}, {300, "2"}, {400, "X"}, {500, "2"}}...)
	s3i.TimeValues([]dbt.TimeValue{{0, "X"}, {100, "3"}, {200, "X"}, {300, "2"}, {400, "X"}, {500, "3"}}...)

	// Create a query engine.
	q := New(dbx)

	s1 := q.Signal("//clk1")
	s2 := q.Signal("//clk2")
	s3 := q.Signal("//clk3")

	// Finds the first timestamp where //clk1=="1", //clk2="2", //clk3="3".
	r := FindFirst(
		func(ts *Timestamp) *Timestamp {
			return s1.FindAfter(ts, "1")
		},
		func(ts *Timestamp) *Timestamp {
			return s2.EqAt(ts, "2")
		},
		func(ts *Timestamp) *Timestamp {
			return s3.EqAt(ts, "3")
		},
	)

	if r.IsNone() || !r.Eq(500) {
		t.Errorf("unexpected: %+v", spew.Sdump(r))
	}

}
