package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
)

// schemaOf reads back everything SQLite records about the database's
// structure: tables, indexes and the statements that made them.
func schemaOf(t *testing.T, dbf *sql.DB) []string {
	t.Helper()
	rows, err := dbf.Query(
		`SELECT type, name, sql FROM sqlite_master ORDER BY type, name`)
	if err != nil {
		t.Fatalf("could not read schema: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var kind, name string
		var stmt sql.NullString
		if err := rows.Scan(&kind, &name, &stmt); err != nil {
			t.Fatalf("could not scan: %v", err)
		}
		got = append(got, kind+" "+name+" "+stmt.String)
	}
	return got
}

// TestBulkLoadMatchesOpenDB is the guarantee the conversion depends on:
// building a database the fast way has to leave exactly what the ordinary
// way would have, indexes included.
func TestBulkLoadMatchesOpenDB(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	plain, err := db.OpenDB(ctx, filepath.Join(dir, "plain.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer plain.Close()

	bulk, err := db.OpenBulk(ctx, filepath.Join(dir, "bulk.db"))
	if err != nil {
		t.Fatalf("OpenBulk: %v", err)
	}
	defer bulk.Close()

	// Before FinishBulk the indexes are deliberately missing.
	if before, after := len(schemaOf(t, bulk)), len(schemaOf(t, plain)); before >= after {
		t.Errorf("bulk database has %v schema entries before FinishBulk, "+
			"want fewer than the %v of a plain one", before, after)
	}
	if err := db.FinishBulk(ctx, bulk); err != nil {
		t.Fatalf("FinishBulk: %v", err)
	}

	want, got := schemaOf(t, plain), schemaOf(t, bulk)
	if len(want) != len(got) {
		t.Fatalf("schema differs:\nplain: %v\nbulk:  %v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("schema entry %v differs:\nplain: %v\nbulk:  %v",
				i, want[i], got[i])
		}
	}
}

// TestInserterRoundTrip covers the batch boundary: the batched statement
// fires every 128 rows, so a count either side of a multiple of that
// exercises both the full-batch path and the single-row remainder.
func TestInserterRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{0, 1, 127, 128, 129, 256, 300} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			dbf, err := db.OpenBulk(ctx, filepath.Join(t.TempDir(), "x.db"))
			if err != nil {
				t.Fatalf("OpenBulk: %v", err)
			}
			defer dbf.Close()
			tx, err := dbf.Begin()
			if err != nil {
				t.Fatalf("Begin: %v", err)
			}
			ins, err := db.Prepare(ctx, tx)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if err := ins.AddSignal(ctx, "/s", vcd.VarKindWire, "!", 1); err != nil {
				t.Fatalf("AddSignal: %v", err)
			}
			for i := 0; i < n; i++ {
				if err := ins.AddValue(ctx, uint64(i), "!", strconv.Itoa(i)); err != nil {
					t.Fatalf("AddValue: %v", err)
				}
			}
			if err := ins.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("Commit: %v", err)
			}
			// Every row must be present, in the order it was added.
			rows, err := dbf.Query(`SELECT Timestamp, Value FROM Svalues ORDER BY Id`)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			defer rows.Close()
			var count int
			for rows.Next() {
				var ts uint64
				var value string
				if err := rows.Scan(&ts, &value); err != nil {
					t.Fatalf("Scan: %v", err)
				}
				if ts != uint64(count) || value != strconv.Itoa(count) {
					t.Fatalf("row %v is (%v, %q), want (%v, %q)",
						count, ts, value, count, strconv.Itoa(count))
				}
				count++
			}
			if count != n {
				t.Errorf("got %v rows, want %v", count, n)
			}
		})
	}
}


// TestFinishBulkIsIdempotent covers finishing a load into a database that
// already carries its indexes, which happens when OpenBulk is pointed at a
// file that already exists.
func TestFinishBulkIsIdempotent(t *testing.T) {
	ctx := context.Background()
	name := filepath.Join(t.TempDir(), "twice.db")
	dbf, err := db.OpenBulk(ctx, name)
	if err != nil {
		t.Fatalf("OpenBulk: %v", err)
	}
	defer dbf.Close()
	for i := 0; i < 2; i++ {
		if err := db.FinishBulk(ctx, dbf); err != nil {
			t.Fatalf("FinishBulk %v: %v", i, err)
		}
	}
}
