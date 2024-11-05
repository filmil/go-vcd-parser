package dbq

import (
	"context"
	"testing"

	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/dbt"
	"github.com/filmil/go-vcd-parser/vcd"
)

func TestBasic(t *testing.T) {
	t.Parallel()
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

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
		TimeValues([]dbt.TimeValue{{0, "0"}, {100, "1"}}...)

	// Create a query engine.
	q := New(dbx, ctx)

	// Demo query: just find first value of the signal.
	s := q.Signal("//clk")
	ts := s.FindFirst("1")

	// Crude examination.
	if ts.Error() != nil {
		t.Fatalf("in timestamp: %v", ts.Error())
	}
	if ts.IsNone() || ts.T() != 100 {
		t.Errorf("mismatch: %+v:", ts)
	}

}
