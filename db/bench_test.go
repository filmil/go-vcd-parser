package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/filmil/go-vcd-parser/db"
)

// This file records what each decision in the bulk write path is worth.
// Measured on an 8-core Linux box, 50,000 rows per iteration:
//
//	InsertExecPerRow          ~64k rows/s   what the conversion used to do
//	InsertPreparedPerRow     ~120k rows/s   preparing the statement once
//	InsertBatchedIndexed     ~120k rows/s   batching, indexes still present
//	InsertBulk               ~490k rows/s   batched, indexes built afterwards
//	InsertBulkNoIndex        ~835k rows/s   the same without the index build
//
// The pair in the middle is the interesting one: with the indexes present,
// batching buys nothing, because maintaining them row by row costs more
// than the driver does. Batching only pays once the indexes are deferred.
//
// benchRows is how many value rows each iteration writes. A conversion of
// a realistic dump writes hundreds of thousands, so the per-row costs this
// file compares are the ones that decide how long it takes.
const benchRows = 50_000

// row generates deterministic but varied rows, so the work is not
// dominated by inserting the same value over and over.
func row(i int) (uint64, string, string) {
	return uint64(i) * 500, fmt.Sprintf("c%d", i%400), fmt.Sprintf("b%b", i%4096)
}

func reportRows(b *testing.B) {
	b.ReportMetric(float64(benchRows*b.N)/b.Elapsed().Seconds(), "rows/s")
}

// openAt makes a fresh database file per iteration; opener chooses between
// the general path and the bulk-load path.
func openAt(b *testing.B, i int, opener func(context.Context, string) (*sql.DB, error)) *sql.DB {
	b.Helper()
	name := filepath.Join(b.TempDir(), fmt.Sprintf("bench.%d.db", i))
	dbf, err := opener(context.Background(), name)
	if err != nil {
		b.Fatalf("could not open: %v", err)
	}
	return dbf
}

// BenchmarkInsertExecPerRow is the shape the conversion used before: one
// tx.ExecContext per row, which makes the driver parse the same INSERT
// once for every row written.
func BenchmarkInsertExecPerRow(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dbf := openAt(b, i, db.OpenDB)
		tx, err := dbf.Begin()
		if err != nil {
			b.Fatalf("could not begin: %v", err)
		}
		for j := 0; j < benchRows; j++ {
			ts, code, value := row(j)
			if err := db.AddValue(ctx, tx, ts, code, value); err != nil {
				b.Fatalf("could not add: %v", err)
			}
		}
		if err := tx.Commit(); err != nil {
			b.Fatalf("could not commit: %v", err)
		}
		dbf.Close()
	}
	b.StopTimer()
	reportRows(b)
}

// BenchmarkInsertPreparedPerRow prepares the statement once but still
// executes it a row at a time, so the gap to BenchmarkInsertExecPerRow is
// what repeated statement preparation costs, and the gap to
// BenchmarkInsertBatchedIndexed is what batching buys on top.
func BenchmarkInsertPreparedPerRow(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dbf := openAt(b, i, db.OpenDB)
		tx, err := dbf.Begin()
		if err != nil {
			b.Fatalf("could not begin: %v", err)
		}
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO Svalues(Timestamp, Code, Value) VALUES (?,?,?)`)
		if err != nil {
			b.Fatalf("could not prepare: %v", err)
		}
		for j := 0; j < benchRows; j++ {
			ts, code, value := row(j)
			if _, err := stmt.ExecContext(ctx, ts, code, value); err != nil {
				b.Fatalf("could not add: %v", err)
			}
		}
		if err := stmt.Close(); err != nil {
			b.Fatalf("could not close: %v", err)
		}
		if err := tx.Commit(); err != nil {
			b.Fatalf("could not commit: %v", err)
		}
		dbf.Close()
	}
	b.StopTimer()
	reportRows(b)
}

// BenchmarkInsertBulk is the conversion's path: prepared and batched
// statements, no journal, and indexes built after the rows are in.
func BenchmarkInsertBulk(b *testing.B) {
	benchPrepared(b, db.OpenBulk, db.FinishBulk)
}

// BenchmarkInsertBulkNoIndex isolates the index build by leaving it out,
// so the difference against BenchmarkInsertBulk is what FinishBulk costs.
func BenchmarkInsertBulkNoIndex(b *testing.B) {
	benchPrepared(b, db.OpenBulk, nil)
}

// BenchmarkInsertBatchedIndexed uses the batched statements but with the
// indexes present and journalling on, so the difference against
// BenchmarkInsertBulk is what deferring the indexes and dropping the
// journal are worth.
func BenchmarkInsertBatchedIndexed(b *testing.B) {
	benchPrepared(b, db.OpenDB, nil)
}

func benchPrepared(b *testing.B,
	opener func(context.Context, string) (*sql.DB, error),
	finish func(context.Context, *sql.DB) error) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dbf := openAt(b, i, opener)
		tx, err := dbf.Begin()
		if err != nil {
			b.Fatalf("could not begin: %v", err)
		}
		ins, err := db.Prepare(ctx, tx)
		if err != nil {
			b.Fatalf("could not prepare: %v", err)
		}
		for j := 0; j < benchRows; j++ {
			ts, code, value := row(j)
			if err := ins.AddValue(ctx, ts, code, value); err != nil {
				b.Fatalf("could not add: %v", err)
			}
		}
		if err := ins.Close(); err != nil {
			b.Fatalf("could not close: %v", err)
		}
		if err := tx.Commit(); err != nil {
			b.Fatalf("could not commit: %v", err)
		}
		if finish != nil {
			if err := finish(ctx, dbf); err != nil {
				b.Fatalf("could not finish: %v", err)
			}
		}
		dbf.Close()
	}
	b.StopTimer()
	reportRows(b)
}
