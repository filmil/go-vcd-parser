package dbq

import (
	"context"
	"database/sql"
)

type Timestamp struct {
	ts   *uint64
	err  error
	name string
	val  string
}

func (self Timestamp) Error() error {
	return self.err
}

func (self Timestamp) IsNone() bool {
	return self.ts == nil
}

// REQUIRES: self.IsNone() == false
func (self Timestamp) T() uint64 {
	return *self.ts
}

type Instance struct {
	db  *sql.DB
	ctx context.Context
}

func New(db *sql.DB, ctx context.Context) *Instance {
	return &Instance{
		db:  db,
		ctx: ctx,
	}
}

func (self *Instance) Signal(name string) *Signal {
	return &Signal{
		i: self,
	}
}

type Signal struct {
	i    *Instance
	name string
}

func (self *Signal) FindFirst(val string) *Timestamp {
	ret := &Timestamp{
		name: self.name,
		val:  val,
	}
	ctx, cf := context.WithCancel(self.i.ctx)
	defer cf()
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
        SELECT MIN(Svalues.Timestamp)
        FROM Svalues
        INNER JOIN Signals
        ON Svalues.Code = Signals.Code
        WHERE Signals.Name = ?
          AND Svalues.Value = ?;
        `,
		self.name, val)
	if err := tx.Commit(); err != nil {
		ret.err = err
		return ret
	}
	if rows.Next() {
		var ts uint64
		err := rows.Scan(&ts)
		if err != nil {
			ret.err = err
			return ret
		}
		ret.ts = &ts
	}
	return ret
}
