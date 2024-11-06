package dbq

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

var (
	// TimestampInfty is a timestamp that is larger than any conceivable timestamp.
	TimestampInfty = Timestamp{
		ts: ptr[uint64](math.MaxUint64),
	}
	// TimestampZero is a timestamp at time zero.
	TimestampZero = Timestamp{
		ts: ptr[uint64](0),
	}
)

type Timestamp struct {
	ts   *uint64
	err  error
	name string
	val  string
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
		ret.err = err
		return ret
	}
	rows, err := tx.QueryContext(ctx, q, self.name, val, t.T())
	if rows.Next() {
		var ts uint64
		err := rows.Scan(&ts)
		if err != nil {
			ret.err = err
		}
		ret.ts = &ts
	} else {
		if rows.Err() != nil {
			ret.err = rows.Err()
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
	fmt.Printf("name: %v; val: %v\n", self.name, val)
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
	ts, err := scan1[uint64](rows)
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
	ts, val, err := scan2[uint64, string](rows)
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
	ts, val, err := scan2[uint64, string](rows)
	if err != nil {
		ret.err = err
		return &ret
	}
	ret.ts = ts
	ret.val = *val

	return &ret
}

func scan1[T any](rows *sql.Rows) (*T, error) {
	if !rows.Next() {
		return nil, fmt.Errorf("not found")
	}
	var ret T
	if err := rows.Scan(&ret); err != nil {
		return nil, fmt.Errorf("not scannable: %w", err)
	}
	return &ret, nil
}

func scan2[T any, U any](rows *sql.Rows) (*T, *U, error) {
	if !rows.Next() {
		return nil, nil, fmt.Errorf("not found")
	}
	var (
		ret1 T
		ret2 U
	)
	if err := rows.Scan(&ret1, &ret2); err != nil {
		return nil, nil, fmt.Errorf("not scannable: %w", err)
	}
	return &ret1, &ret2, nil
}
