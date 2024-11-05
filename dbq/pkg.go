package dbq

import (
	"context"
	"database/sql"
	"fmt"
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
	if rows.Next() {
		var ts uint64
		err := rows.Scan(&ts)
		fmt.Printf("scan: %v\n", ts)
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
