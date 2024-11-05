package dbt

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
)

func TestBasic(t *testing.T) {
	t.Parallel()
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	dbx, err := db.OpenDB(ctx, NewMemDB())
	if err != nil {
		t.Fatalf("could not open DB: %v", err)
	}

	i := New(dbx, ctx)
	clk := i.Signal("//clk", vcd.VarKindLogic, 1)
	clk.TimeValues([]TimeValue{
		{0, "0"},
		{100, "1"},
		{200, "0"},
		{300, "1"},
		{400, "0"},
	}...)

	res, err := dbx.QueryContext(ctx, `select Name from Signals;`)
	if err != nil {
		t.Fatalf("could not exec query: %v", err)
	}
	var s string
	res.Next()
	if err = res.Scan(&s); err != nil {
		t.Fatalf("could not scan: %v", err)
	}
	if s != "//clk" {
		t.Errorf("signal name mismatch: %q", s)
	}
}

func rows(res *sql.Rows) []string {
	var ret []string
	for res.Next() {
		var s string
		if err := res.Scan(&s); err != nil {
			panic(fmt.Sprintf("while scanning signal names: %v", err))
		}
		ret = append(ret, s)
	}
	return ret
}

func TestTwoSignals(t *testing.T) {
	t.Parallel()
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	dbx, err := db.OpenDB(ctx, NewMemDB())
	if err != nil {
		t.Fatalf("could not open DB: %v", err)
	}

	i := New(dbx, ctx)

	// Example of multiple signal addition.
	i.
		Signal("//clk", vcd.VarKindLogic, 1).
		TimeValues([]TimeValue{
			{0, "0"}, {100, "1"}, {200, "0"}, {300, "1"}, {400, "0"}}...).
		Signal("//clk2", vcd.VarKindLogic, 1).
		TimeValues([]TimeValue{
			{0, "0"}, {100, "1"}, {200, "0"}, {300, "1"}, {400, "0"}}...)

	res, err := dbx.QueryContext(ctx, `SELECT Name FROM Signals ORDER BY Name;`)
	if err != nil {
		t.Fatalf("could not exec query: %v", err)
	}
	r := rows(res)
	e := []string{"//clk", "//clk2"}
	if !reflect.DeepEqual(r, e) {
		t.Errorf("mismatch: actual: \n%+v\nexpected:\n%+v", spew.Sdump(r), spew.Sdump(e))
	}
}
