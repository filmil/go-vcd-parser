package cvt_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/filmil/go-vcd-parser/cvt"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/dbt"
	"github.com/filmil/go-vcd-parser/vcd"
)

// vcdFiles are small but complete dumps: a header with nested scopes and
// vars, then timestamps, a dump block and plain value changes.
var vcdFiles = []string{
	`$timescale 1 ns $end
$scope module top $end
$var wire 1 ! clk $end
$var wire 8 " data[7:0] $end
$scope module inner $end
$var real 64 # freq $end
$upscope $end
$upscope $end
$enddefinitions $end
#0
$dumpvars
0!
b00000000 "
r1.5 #
$end
#10
1!
b00000001 "
#20
0!
x!
`,
	`$enddefinitions $end
#0
0V#
b0 #0
srx_get_start_bit ^
`,
}

// dump reads every row back, in insertion order, as comparable text.
func dump(t *testing.T, dbf *sql.DB) string {
	t.Helper()
	var b strings.Builder
	rows, err := dbf.Query(`SELECT Name, Type, Code, Size FROM Signals ORDER BY Name`)
	if err != nil {
		t.Fatalf("could not query signals: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, typ, code string
		var size int
		if err := rows.Scan(&name, &typ, &code, &size); err != nil {
			t.Fatalf("could not scan signal: %v", err)
		}
		fmt.Fprintf(&b, "signal %q %v %q %v\n", name, typ, code, size)
	}
	vals, err := dbf.Query(`SELECT Id, Timestamp, Code, Value FROM Svalues ORDER BY Id`)
	if err != nil {
		t.Fatalf("could not query values: %v", err)
	}
	defer vals.Close()
	for vals.Next() {
		var id, ts uint64
		var code, value string
		if err := vals.Scan(&id, &ts, &code, &value); err != nil {
			t.Fatalf("could not scan value: %v", err)
		}
		fmt.Fprintf(&b, "value %v %v %q %q\n", id, ts, code, value)
	}
	return b.String()
}

func newDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	dbf, err := db.OpenDB(ctx, dbt.NewMemDB())
	if err != nil {
		t.Fatalf("could not open database: %v", err)
	}
	t.Cleanup(func() { dbf.Close() })
	return dbf
}

// TestConvertStreamMatchesConvert is what keeps the two conversion entry
// points honest: streaming a file and replaying its parse tree have to
// write exactly the same rows, in the same order.
func TestConvertStreamMatchesConvert(t *testing.T) {
	ctx := context.Background()
	for i, input := range vcdFiles {
		t.Run(fmt.Sprintf("file %v", i), func(t *testing.T) {
			treeDB := newDB(t, ctx)
			file, err := vcd.NewParser[vcd.File]().Parse("(test)", strings.NewReader(input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := cvt.Convert(ctx, file, treeDB); err != nil {
				t.Fatalf("Convert: %v", err)
			}

			streamDB := newDB(t, ctx)
			if err := cvt.ConvertStream(ctx, "(test)", strings.NewReader(input), streamDB); err != nil {
				t.Fatalf("ConvertStream: %v", err)
			}

			want, got := dump(t, treeDB), dump(t, streamDB)
			if want != got {
				t.Errorf("rows differ:\nConvert:\n%v\nConvertStream:\n%v", want, got)
			}
			if want == "" {
				t.Error("no rows written")
			}
		})
	}
}

// TestConvertStreamCommitsAcrossTransactions exercises the transaction
// rollover, which only happens past MaxTx rows.
func TestConvertStreamCommitsAcrossTransactions(t *testing.T) {
	ctx := context.Background()
	old := cvt.MaxTx
	cvt.MaxTx = 3
	t.Cleanup(func() { cvt.MaxTx = old })

	dbf := newDB(t, ctx)
	if err := cvt.ConvertStream(ctx, "(test)", strings.NewReader(vcdFiles[0]), dbf); err != nil {
		t.Fatalf("ConvertStream: %v", err)
	}
	var n int
	if err := dbf.QueryRow(`SELECT count(*) FROM Svalues`).Scan(&n); err != nil {
		t.Fatalf("could not count: %v", err)
	}
	// Three in the $dumpvars block, two at #10, two at #20.
	if want := 7; n != want {
		t.Errorf("got %v value rows, want %v", n, want)
	}
}

// TestTimescaleMeta checks the row that says which unit the timestamps
// of Svalues count. Without it a reader has the numbers and no way to
// turn them into seconds.
func TestTimescaleMeta(t *testing.T) {
	ctx := context.Background()
	dbf := newDB(t, ctx)
	file, err := vcd.NewParser[vcd.File]().Parse("(test)", strings.NewReader(vcdFiles[0]))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := cvt.Convert(ctx, file, dbf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	tx, err := dbf.Begin()
	if err != nil {
		t.Fatalf("could not begin: %v", err)
	}
	defer tx.Commit()
	for _, want := range []struct{ key, value string }{
		{"generator", "go-vcd-parser"},
		{"timescale", "1ns"},
		{"timescale_seconds", "1e-09"},
	} {
		got, ok, err := db.GetMeta(ctx, tx, want.key)
		if err != nil {
			t.Fatalf("could not read %q: %v", want.key, err)
		}
		if !ok || got != want.value {
			t.Errorf("%s is %q, %v; want %q", want.key, got, ok, want.value)
		}
	}
}
