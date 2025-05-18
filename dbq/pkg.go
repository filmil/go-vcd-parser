package dbq

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

func (self Timestamp) D() time.Duration {
	// Assumes sim resolution of 1s.
	return time.Duration(self.T()) * time.Nanosecond / 1000
}

type Instance struct {
	db *sql.DB
}

func New(db *sql.DB) *Instance {
	return &Instance{
		db: db,
	}
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

func (self *Signal) findSignal(t *Timestamp, val string, q string) *Timestamp {
	ret := &Timestamp{
		name: self.name,
		val:  val,
	}
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
	ret := &Timestamp{
		name: self.name,
		val:  val,
	}
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
	var ret Timestamp
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
	var ret Timestamp
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
