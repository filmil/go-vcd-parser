package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/filmil/go-vcd-parser/vcd"
)

// bulkParams are the connection settings for building a database from
// scratch. A conversion writes a file that does not exist yet and is
// worthless if the run fails, so there is nothing for the rollback journal
// or for fsync to protect: both are turned off for the duration.
//
// They are passed in the DSN rather than run as PRAGMA statements because
// a *sql.DB is a pool, and a PRAGMA would only apply to whichever
// connection happened to run it.
//
// The page cache is deliberately small. A load writes each page once and
// never reads it back, so a large cache only inflates the resident set:
// measured over tb.vcd, going from 2 MB to 200 MB of cache costs 57 MB of
// RSS and saves no time at all. See BenchmarkCacheSize.
// It is a var rather than a const only so that BenchmarkCacheSize can
// vary it; nothing else assigns to it.
var bulkParams = "_journal_mode=OFF&_synchronous=OFF&_cache_size=-2000"

// OpenBulk creates a new database for a bulk load: tables but no indexes,
// and no journalling. Call FinishBulk once the rows are in to build the
// indexes and leave a database identical to one OpenDB would have made.
func OpenBulk(ctx context.Context, name string) (*sql.DB, error) {
	needsInit, err := CreateDBFile(name)
	if err != nil {
		return nil, fmt.Errorf("could not create DB file: %q:\n\t%v", name, err)
	}
	dbf, err := sql.Open(SqliteDriver, withParams(name, bulkParams))
	if err != nil {
		return nil, fmt.Errorf("could not open %q: %w", name, err)
	}
	// One writer: the load is serial, and a second connection would only
	// contend for the write lock.
	dbf.SetMaxOpenConns(1)
	if !needsInit {
		return dbf, nil
	}
	if err := inTx(ctx, dbf, CreateTables); err != nil {
		dbf.Close()
		return nil, err
	}
	return dbf, nil
}

// FinishBulk builds the indexes that OpenBulk left out.
func FinishBulk(ctx context.Context, dbf *sql.DB) error {
	return inTx(ctx, dbf, CreateIndexes)
}

func inTx(ctx context.Context, dbf *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	tx, err := dbf.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin: %w", err)
	}
	if err := fn(ctx, tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit: %w", err)
	}
	return nil
}

// withParams appends DSN query parameters to a database name, keeping any
// the caller already supplied.
func withParams(name, params string) string {
	if strings.HasPrefix(name, "file:") || strings.Contains(name, "?") {
		sep := "?"
		if strings.Contains(name, "?") {
			sep = "&"
		}
		return name + sep + params
	}
	return "file:" + url.PathEscape(name) + "?" + params
}

// batchRows is how many value rows go into one INSERT. Preparing the
// statement once removes the per-row parse; batching removes most of what
// database/sql itself costs per Exec, which is what dominates once the
// parse is gone. 128 rows is 384 bound parameters, comfortably inside
// SQLite's limit of 999.
const batchRows = 128

// Inserter writes rows through prepared, batched statements instead of
// letting the driver prepare and finalise one statement per row. At
// hundreds of thousands of rows that difference dominates the conversion.
//
// An Inserter belongs to one transaction. Close it before committing.
type Inserter struct {
	ctx context.Context

	signals *sql.Stmt
	// batch inserts batchRows value rows at once; one inserts a single
	// row, for the remainder at the end of a batch.
	batch *sql.Stmt
	one   *sql.Stmt

	// args holds the bound parameters of the rows buffered so far.
	args []any
}

// Prepare readies the insert statements on tx.
func Prepare(ctx context.Context, tx *sql.Tx) (*Inserter, error) {
	i := &Inserter{ctx: ctx, args: make([]any, 0, batchRows*3)}
	var err error
	if i.signals, err = tx.PrepareContext(ctx,
		`INSERT INTO Signals(Name, Type, Code, Size) VALUES (?, ?, ?, ?)`); err != nil {
		return nil, fmt.Errorf("db.Prepare: signals: %w", err)
	}
	if i.one, err = tx.PrepareContext(ctx, valuesInsert(1)); err != nil {
		i.Close()
		return nil, fmt.Errorf("db.Prepare: value: %w", err)
	}
	if i.batch, err = tx.PrepareContext(ctx, valuesInsert(batchRows)); err != nil {
		i.Close()
		return nil, fmt.Errorf("db.Prepare: value batch: %w", err)
	}
	return i, nil
}

// valuesInsert builds an INSERT carrying n rows of placeholders.
func valuesInsert(n int) string {
	var b strings.Builder
	b.WriteString(`INSERT INTO Svalues(Timestamp, Code, Value) VALUES `)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("(?,?,?)")
	}
	return b.String()
}

// AddSignal writes one signal row.
func (i *Inserter) AddSignal(ctx context.Context,
	name string, kindCode vcd.VarKindCode, code string, size int) error {
	if _, err := i.signals.ExecContext(ctx, name, kindCode.Int(), code, size); err != nil {
		return fmt.Errorf("db.Inserter.AddSignal: %q: %w", name, err)
	}
	return nil
}

// AddValue buffers one value change row, writing the batch once it is
// full. Rows reach the table in the order they were added.
func (i *Inserter) AddValue(ctx context.Context,
	timestamp uint64, code, value string) error {
	i.args = append(i.args, timestamp, code, value)
	if len(i.args) < batchRows*3 {
		return nil
	}
	return i.flushBatch()
}

func (i *Inserter) flushBatch() error {
	if _, err := i.batch.ExecContext(i.ctx, i.args...); err != nil {
		return fmt.Errorf("db.Inserter: could not add values: %w", err)
	}
	i.args = i.args[:0]
	return nil
}

// Flush writes any buffered rows. It must be called before the enclosing
// transaction is committed; Close does it for you.
func (i *Inserter) Flush() error {
	if i.one == nil {
		return nil // Prepare did not get far enough to buffer anything
	}
	if len(i.args) == batchRows*3 {
		return i.flushBatch()
	}
	// The remainder of a partly filled batch goes in one row at a time.
	// This runs once per transaction, so its cost does not matter.
	for off := 0; off+3 <= len(i.args); off += 3 {
		if _, err := i.one.ExecContext(i.ctx,
			i.args[off], i.args[off+1], i.args[off+2]); err != nil {
			return fmt.Errorf("db.Inserter: could not add value: %w", err)
		}
	}
	i.args = i.args[:0]
	return nil
}

// Close flushes any buffered rows and releases the prepared statements.
func (i *Inserter) Close() error {
	err := i.Flush()
	for _, stmt := range []*sql.Stmt{i.signals, i.one, i.batch} {
		if stmt == nil {
			continue
		}
		if cerr := stmt.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
