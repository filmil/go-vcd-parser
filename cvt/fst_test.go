// SPDX-License-Identifier: Apache-2.0

package cvt_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/filmil/go-vcd-parser/cvt"
	"github.com/filmil/go-vcd-parser/db"
)

// TestConvertFST reads the checked-in dump from //bin/fstgen and checks the
// rows it produces. It covers the path a user takes with
// `vcdcvt -in dump.fst -format sqlite`: the FST reader, the sink, and the
// bulk load, end to end.
func TestConvertFST(t *testing.T) {
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "small.db")

	dbx, err := db.OpenBulk(ctx, out)
	if err != nil {
		t.Fatalf("db.OpenBulk: %v", err)
	}
	defer dbx.Close()

	if err := cvt.ConvertFST(ctx, "testdata/small.fst", dbx); err != nil {
		t.Fatalf("cvt.ConvertFST: %v", err)
	}
	if err := db.FinishBulk(ctx, dbx); err != nil {
		t.Fatalf("db.FinishBulk: %v", err)
	}

	t.Run("signals", func(t *testing.T) {
		// The names carry the whole scope path, as they do for a VCD
		// dump, and the bit range stays part of the identifier.
		want := map[string]int{
			"//top/clk":        1,
			"//top/count[3:0]": 4,
			"//top/sub/ready":  1,
		}
		rows, err := dbx.Query(`SELECT Name, Size FROM Signals;`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()
		got := map[string]int{}
		for rows.Next() {
			var n string
			var s int
			if err := rows.Scan(&n, &s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got[n] = s
		}
		if len(got) != len(want) {
			t.Fatalf("got %d signals %v, want %d %v", len(got), got, len(want), want)
		}
		for n, s := range want {
			if got[n] != s {
				t.Errorf("signal %q size = %d, want %d", n, got[n], s)
			}
		}
	})

	t.Run("values", func(t *testing.T) {
		rows, err := dbx.Query(
			`SELECT s.Name, v.Timestamp, v.Value FROM Svalues v
			 JOIN Signals s ON s.Code = v.Code
			 ORDER BY v.Timestamp, s.Name;`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()

		var got []string
		for rows.Next() {
			var n, v string
			var ts uint64
			if err := rows.Scan(&n, &ts, &v); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, fmt.Sprintf("%d %s=%s", ts, n, v))
		}

		want := []string{
			"0 //top/clk=0",
			"0 //top/count[3:0]=0000",
			"0 //top/sub/ready=x",
			"10 //top/clk=1",
			"10 //top/count[3:0]=0001",
			"20 //top/clk=0",
			"20 //top/sub/ready=1",
			"30 //top/clk=1",
			"30 //top/count[3:0]=0010",
			"30 //top/sub/ready=0",
		}
		if len(got) != len(want) {
			t.Fatalf("got %d rows, want %d\n got: %v\nwant: %v", len(got), len(want), got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("row %d: got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("timescale", func(t *testing.T) {
		// The FST exponent -12 has to arrive as the VCD spelling.
		for k, want := range map[string]string{
			"timescale":         "1ps",
			"timescale_seconds": "1e-12",
		} {
			var got string
			if err := dbx.QueryRow(`SELECT Value FROM Meta WHERE Key = ?;`, k).Scan(&got); err != nil {
				t.Fatalf("meta %q: %v", k, err)
			}
			if got != want {
				t.Errorf("meta %q = %q, want %q", k, got, want)
			}
		}
	})
}
