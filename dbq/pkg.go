package dbq

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/golang/glog"
)

var (
	// TimestampInfty is a timestamp that is larger than any conceivable timestamp.
	TimestampInfty = Timestamp{
		ts: ptr[uint64](math.MaxUint64),
	}
	TimestampNone = ptr(Timestamp{})

	// TimestampZero is a timestamp at time zero.
	TimestampZero = Timestamp{
		ts: ptr[uint64](0),
	}

	testDbName string
	testDb     *sql.DB
)

func init() {
	flag.StringVar(&testDbName, "test-db-name", "", "The test db name.")
}

// GetTestDB Obtains a test database for this test case.  Only one database is
// opened per a test package.
func GetTestDB() (*sql.DB, context.Context, error) {
	ctx := context.Background()
	if testDbName == "" {
		return nil, nil, fmt.Errorf("No test db name. Start test with arg --test-db-name=...")
	}
	if testDb != nil {
		return testDb, ctx, nil
	}
	runfiles_dir := os.Getenv("RUNFILES_DIR")
	testDb, err := db.OpenDB(ctx, filepath.Join(runfiles_dir, testDbName))
	return testDb, ctx, err
}

type Timestamp struct {
	ts   *uint64
	err  error
	name string
	val  string

	// tickFs is how many femtoseconds one raw timestamp counts, taken
	// from the Meta table of the database the timestamp came from. Zero
	// means the database did not say, and DefaultTickFs is used.
	tickFs int64
}

// Pretty-prints a Timestamp.
func (self Timestamp) String() string {
	return fmt.Sprintf("%v on %q", self.D(), self.name)
}

func (self Timestamp) Eq(ts uint64) bool {
	if self.IsNone() {
		return false
	}
	return *(self.ts) == ts
}

func ptr[T any](v T) *T {
	return &v
}

func (self Timestamp) Error() error {
	return self.err
}

func (self Timestamp) IsNone() bool {
	return self.ts == nil
}

// PROVIDES: Only valid if ValueAt() != ""
func (self Timestamp) ValueAt() string {
	return self.val
}

// REQUIRES: self.IsNone() == false
func (self Timestamp) T() uint64 {
	return *self.ts
}

// DefaultTickFs is the resolution assumed for a database whose Meta table
// does not record one: one picosecond per tick. Databases written before
// Meta existed are read exactly as they were before.
const DefaultTickFs = 1000

// Fs returns the timestamp in femtoseconds, which is exact for every
// timescale a VCD can declare.
//
// REQUIRES: self.IsNone() == false
func (self Timestamp) Fs() int64 {
	return int64(self.T()) * self.resolution()
}

// D returns the timestamp as a duration.
//
// A time.Duration counts whole nanoseconds, so a dump finer than that --
// and 1fs is common -- loses the sub-nanosecond part here. Use Fs when
// that matters.
//
// REQUIRES: self.IsNone() == false
func (self Timestamp) D() time.Duration {
	return time.Duration(self.Fs() / 1_000_000)
}

// resolution is the femtoseconds per tick to use for this timestamp.
func (self Timestamp) resolution() int64 {
	if self.tickFs == 0 {
		return DefaultTickFs
	}
	return self.tickFs
}

type Instance struct {
	db *sql.DB

	// tickFs is how many femtoseconds one raw timestamp counts. Read
	// once here rather than per query, since it cannot change.
	tickFs int64
}

func New(dbx *sql.DB) *Instance {
	return &Instance{
		db:     dbx,
		tickFs: readTickFs(dbx),
	}
}

// readTickFs reads the simulation resolution out of the Meta table.
//
// A database written before Meta existed, or by a writer that does not
// record a timescale, reports DefaultTickFs, so such a database keeps
// behaving exactly as it did.
func readTickFs(dbx *sql.DB) int64 {
	ctx := context.Background()
	tx, err := dbx.Begin()
	if err != nil {
		glog.V(1).Infof("dbq: could not read the timescale: %v", err)
		return DefaultTickFs
	}
	defer tx.Rollback()
	text, ok, err := db.GetMeta(ctx, tx, "timescale_seconds")
	if err != nil || !ok {
		glog.V(1).Infof("dbq: no timescale recorded, assuming %v fs per tick",
			DefaultTickFs)
		return DefaultTickFs
	}
	sec, err := strconv.ParseFloat(text, 64)
	if err != nil || sec <= 0 {
		glog.V(1).Infof("dbq: timescale %q is not a positive number", text)
		return DefaultTickFs
	}
	// Every VCD timescale is a power of ten times 1, 10 or 100 seconds,
	// and femtoseconds is the finest of them, so this rounds to an exact
	// integer for any real file.
	fs := math.Round(sec * 1e15)
	if fs < 1 {
		glog.V(1).Infof("dbq: timescale %q is finer than a femtosecond", text)
		return DefaultTickFs
	}
	return int64(fs)
}

func (self *Instance) Signal(name string) *Signal {
	return &Signal{
		i:    self,
		name: name,
	}
}

type Signal struct {
	i    *Instance
	name string
}

func (self Signal) String() string {
	return fmt.Sprintf(self.name)
}

func (self Signal) Name() string {
	return self.name
}

// newTimestamp starts a result for this signal, carrying the resolution
// of the database it is being read from.
func (self *Signal) newTimestamp(val string) Timestamp {
	return Timestamp{
		name:   self.name,
		val:    val,
		tickFs: self.i.tickFs,
	}
}

func (self *Signal) findSignal(t *Timestamp, val string, q string) *Timestamp {
	ret := ptr(self.newTimestamp(val))
	if t.IsNone() {
		return ret
	}
	ctx := context.TODO()
	dbx := self.i.db
	tx, err := dbx.Begin()
	if err != nil {
		ret.err = fmt.Errorf("while looking up value %q in signal %q:\n\t%w",
			val, self.name, err)
		return ret
	}
	rows, err := tx.QueryContext(ctx, q, self.name, val, t.T())
	if rows.Next() {
		var ts uint64
		err := rows.Scan(&ts)
		if err != nil {
			ret.err = fmt.Errorf("while looking up value %q in signal %q:\n\t%w",
				val, self.name, err)
		}
		ret.ts = &ts
	} else {
		if rows.Err() != nil {
			ret.err = fmt.Errorf("while looking up value %q in signal %q:\n\t%w",
				val, self.name, rows.Err())
		}
	}
	return ret
}

func (self *Signal) FindBefore(t *Timestamp, val string) *Timestamp {
	return self.findSignal(t, val,
		`
        -- Finds the first matching value before the given timestamp.
        SELECT      MAX(Svalues.Timestamp)
        FROM        Svalues
        INNER JOIN  Signals
        ON          Svalues.Code=Signals.Code
        WHERE       Signals.Name=?
          AND       Svalues.Value=?
          AND       Svalues.Timestamp < ?;
        `,
	)
}

func (self *Signal) FindAfter(t *Timestamp, val string) *Timestamp {
	return self.findSignal(t, val,
		`
        -- Finds first timestamp from the beginning of time at which the given
        -- signal had the specified value.
        SELECT      MIN(Svalues.Timestamp)
        FROM        Svalues
        INNER JOIN  Signals
        ON          Svalues.Code=Signals.Code
        WHERE       Signals.Name=?
          AND       Svalues.Value=?
          AND       Svalues.Timestamp > ?;
        `,
	)
}

type Value struct {
	val *string
	err error
}

func (self Value) Error() error {
	return self.err
}

// REQUIRES self.IsNone() == false.
func (self Value) V() string {
	return *self.val
}

func (self Value) IsNone() bool {
	return self.val == nil
}

func (self *Signal) EqAt(t *Timestamp, v string) *Timestamp {
	if self.ValueAtP(t).V() == v {
		return t
	}
	return nil
}

// ValueAtP returns the value of the signal exactly at the timestamp - including
// when there is a signal change exactly at the timestamp.
func (self *Signal) ValueAtP(t *Timestamp) *Value {
	var ret Value
	ctx := context.TODO()
	dbx := self.i.db
	tx, err := dbx.Begin()
	if err != nil {
		ret.err = err
		return &ret
	}
	rows, err := tx.QueryContext(
		ctx,
		`
        -- Find the value at the most recent transition before the given
        -- timestamp.
        -- TODO: Perhaps introduce a WITH table?
        SELECT      Svalues.Value
        FROM        Svalues INNER JOIN  Signals
        ON          Svalues.Code = Signals.Code
        WHERE       Signals.Name = ?
          AND       Svalues.Timestamp = (
            SELECT      MAX(Svalues.Timestamp)
            FROM        Svalues INNER JOIN  Signals
            ON          Svalues.Code=Signals.Code
            WHERE       Signals.Name=?
              AND       Svalues.Timestamp = ?
          )
        ;
        `,
		self.name, self.name, t.T())
	if rows.Next() {
		var val string
		err := rows.Scan(&val)
		if err != nil {
			ret.err = err
		}
		ret.val = &val
	} else {
		if rows.Err() != nil {
			ret.err = rows.Err()
		} else {
			return self.ValueAt(t)
		}
	}
	return &ret
}

func (self *Signal) ValueAt(t *Timestamp) *Value {
	var ret Value
	ctx := context.TODO()
	dbx := self.i.db
	tx, err := dbx.Begin()
	if err != nil {
		ret.err = err
		return &ret
	}
	rows, err := tx.QueryContext(
		ctx,
		`
        -- Find the value at the most recent transition before the given
        -- timestamp.
        -- TODO: Perhaps introduce a WITH table?
        SELECT      Svalues.Value
        FROM        Svalues INNER JOIN  Signals
        ON          Svalues.Code = Signals.Code
        WHERE       Signals.Name = ?
          AND       Svalues.Timestamp = (
            SELECT      MAX(Svalues.Timestamp)
            FROM        Svalues INNER JOIN  Signals
            ON          Svalues.Code=Signals.Code
            WHERE       Signals.Name=?
              AND       Svalues.Timestamp < ?
          )
        ;
        `,
		self.name, self.name, t.T())
	if rows.Next() {
		var val string
		err := rows.Scan(&val)
		if err != nil {
			ret.err = err
		}
		ret.val = &val
	} else {
		if rows.Err() != nil {
			ret.err = rows.Err()
		}
	}
	return &ret
}

func (self *Signal) FindFirst(val string) *Timestamp {
	ret := ptr(self.newTimestamp(val))
	ctx := context.TODO()
	dbx := self.i.db
	tx, err := dbx.Begin()
	if err != nil {
		ret.err = err
		return ret
	}
	rows, err := tx.QueryContext(
		ctx,
		`
        -- Finds first timestamp from the beginning of time at which the given
        -- signal had the specified value.
        SELECT      MIN(Svalues.Timestamp)
        FROM        Svalues
        INNER JOIN  Signals
        ON          Svalues.Code=Signals.Code
        WHERE       Signals.Name=?
          AND       Svalues.Value=?;
        `,
		self.name, val)
	if err != nil {
		ret.err = err
		return ret
	}
	ts, err := db.Scan1[uint64](rows)
	if err != nil {
		ret.err = err
		return ret
	}
	ret.ts = ts
	return ret
}

func (self *Signal) PrevChange(t *Timestamp) *Timestamp {
	ret := self.newTimestamp("")
	ctx := context.TODO()
	dbx := self.i.db
	tx, err := dbx.Begin()
	if err != nil {
		ret.err = err
		return &ret
	}
	rows, err := tx.QueryContext(
		ctx,
		`
        -- Find the value at the most recent transition before the given
        -- timestamp.
        -- TODO: Perhaps introduce a WITH table?
        SELECT      Svalues.Timestamp, Svalues.Value
        FROM        Svalues INNER JOIN  Signals
        ON          Svalues.Code = Signals.Code
        WHERE       Signals.Name = ?
          AND       Svalues.Timestamp = (
            SELECT      MAX(Svalues.Timestamp)
            FROM        Svalues INNER JOIN  Signals
            ON          Svalues.Code=Signals.Code
            WHERE       Signals.Name=?
              AND       Svalues.Timestamp < ?
          )
        ;
        `,
		self.name, self.name, t.T())
	if err != nil {
		ret.err = err
		return &ret
	}
	ts, val, err := db.Scan2[uint64, string](rows)
	if err != nil {
		ret.err = err
		return &ret
	}
	ret.ts = ts
	ret.val = *val

	return &ret
}

// NextChange finds the *next* timestamp at which the signal changes value,
// starting from the given timestamp `t`.
func (self *Signal) NextChange(t *Timestamp) *Timestamp {
	ret := self.newTimestamp("")
	ctx := context.TODO()
	dbx := self.i.db
	tx, err := dbx.Begin()
	if err != nil {
		ret.err = err
		return &ret
	}
	rows, err := tx.QueryContext(
		ctx,
		`
        SELECT      Svalues.Timestamp, Svalues.Value
        FROM        Svalues INNER JOIN  Signals
        ON          Svalues.Code = Signals.Code
        WHERE       Signals.Name = ?
          AND       Svalues.Timestamp = (
            SELECT      MIN(Svalues.Timestamp)
            FROM        Svalues INNER JOIN  Signals
            ON          Svalues.Code=Signals.Code
            WHERE       Signals.Name=?
              AND       Svalues.Timestamp > ?
          )
        ;
        `,
		self.name, self.name, t.T())
	if err != nil {
		ret.err = err
		return &ret
	}
	ts, val, err := db.Scan2[uint64, string](rows)
	if err != nil {
		ret.err = err
		return &ret
	}
	ret.ts = ts
	ret.val = *val

	return &ret
}

// FindTsFn is a timestamp-based function.
type FindTsFn func(*Timestamp) *Timestamp

// / FindFirst finds a timestamp
func FindFirst(fns ...FindTsFn) *Timestamp {
	return FindFirstFrom(&TimestampZero, fns...)
}

// / FindFirstFrom finds a timestamp matching the sequence of predicates `fns`.
func FindFirstFrom(start *Timestamp, fns ...FindTsFn) *Timestamp {
	var retryTs *Timestamp
	currentTs := start

	for found, k := false, 0; !found; k++ {
		glog.V(3).Infof("-------------\n")
		retryTs = currentTs
		var j int
		for i, fn := range fns {
			fmt.Printf("i=%v: applying to: %+v\n", i, currentTs)
			currentTs = fn(currentTs)
			if currentTs == nil || currentTs.IsNone() {
				if i == 0 {
					// Nothing was found, return None.
					glog.V(3).Infof("nothing found sigh.\n")
					retryTs = currentTs
					goto exit
				}
				// Wasn't found, restart from retryTs.
				currentTs = retryTs
				glog.V(3).Infof("i=%v: not found restarting from: %+v\n", i, currentTs)
				break
			} else {
				if i == 0 {
					retryTs = currentTs
				}
				glog.V(3).Infof("i=%v FOUND: %+v\n", i, spew.Sdump(currentTs))
			}
			j = i
		}
		found = j == len(fns)-1 || k > 100
	}
exit:
	return retryTs
}
