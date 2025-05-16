// Package db deals with database access.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
	_ "github.com/mattn/go-sqlite3"
)

const (
	// Pragmas are used to initialize an in-memory database. Useful for tests.
	Pragmas = `?cache=shared&mode=memory`

	// For the time being, use an in-memory database.
	DefaultFilename = `file:test.db` + Pragmas

	// SqliteDriver is the name of the used SQL driver module.
	SqliteDriver = `sqlite3`
)

var (
	// NowFn is a function that retrieves the current time.  Can be overridden
	// in tests.
	NowFn = time.Now
)

// OpenDB opens the database by name, creating with the correct schema if one does not exist.
func OpenDB(ctx context.Context, name string) (*sql.DB, error) {
	needsInit, err := CreateDBFile(name)
	if err != nil {
		return nil, fmt.Errorf("could not create DB file: %q:\n\t%v", name, err)
	}
	db, err := sql.Open(SqliteDriver, name)
	if needsInit {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("error opening the init transaction: %v: %w", name, err)
		}
		if err := CreateSchema(ctx, tx); err != nil {
			return nil, fmt.Errorf("could not create database schema: %v: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit schema creation: %v: %w", name, err)
		}
	}
	return db, nil
}

// CreateDBFile creates the database file if it does not already exist.
// Returns true, if the db schema needs to be created.
func CreateDBFile(name string) (bool, error) {
	const op = "db/CreateDBFile"
	var needsInit bool

	if name == DefaultFilename {
		needsInit = true
	} else {
		_, err := os.Stat(name)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return false, fmt.Errorf("unknown error: %v: %w", name, err)
			}
			// No such file, create it and set for schema creation.
			_, err := os.Create(name)
			if err != nil {
				return false, fmt.Errorf("could not create: %v:\n\t%w", name, err)
			}

			// Add the pragma suffixes
			if !strings.HasSuffix(name, Pragmas) {
				name = fmt.Sprintf("%s%s", name, Pragmas)
			}
			needsInit = true
		}
	}
	return needsInit, nil
}

// CreateSchema schedules a transactional schema creation.
func CreateSchema(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
        CREATE TABLE
            Signals(
                Name STRING PRIMARY KEY,
                Type INTEGER NOT NULL,
                Code STRING NOT NULL,
                Size INTEGER NOT NULL
            );

        CREATE INDEX
            SignalsByCode
        ON
            Signals(Code, Name);

        CREATE TABLE
            Svalues(
                Id INTEGER PRIMARY KEY AUTOINCREMENT,
                Timestamp INTEGER NOT NULL,
                Code STRING NOT NULL,
                Value STRING NOT NULL,
                FOREIGN KEY(Code) REFERENCES Signals(Code)
            );

        CREATE INDEX
            SvaluesByCodeAndTimestamp
        ON
            Svalues(Code, Timestamp, Value);
        `)
	if err != nil {
		return fmt.Errorf("could not create schema: %w", err)
	}
	return nil
}

func AddSignal(ctx context.Context, tx *sql.Tx,
	name string, kindCode vcd.VarKindCode, code string, size int) error {
	glog.V(2).Infof(
		"db/addSignal: name=%q; kindCode=%v code=%q size=%v",
		name, kindCode, code, size)
	_, err := tx.ExecContext(ctx, `
    INSERT INTO Signals(Name, Type, Code, Size)
            VALUES(?, ?, ?, ?);
        `,
		name, kindCode.Int(), code, size)
	if err != nil {
		glog.V(1).Infof(
			"db/addSignal: name=%q; kindCode=%v code=%q size=%v",
			name, kindCode, code, size)
		return fmt.Errorf("db/AddSignal: could not exec tx(%q,%v,%q,%d): %w", name, kindCode, code, size, err)
	}
	return nil
}

func FindSignalByName(ctx context.Context, tx *sql.Tx, name string) *sql.Row {
	glog.V(2).Infof(
		"db/FindSignal: name=%v", name,
	)
	res := tx.QueryRowContext(ctx, `
        SELECT
            Type, Code, Size
        FROM
            Signals
        WHERE
            Name = ?
        LIMIT 1;
    `, name)
	return res
}

func AddValue(ctx context.Context, tx *sql.Tx,
	timestamp uint64, code string, value string) error {
	glog.V(2).Infof("db.AddValue: timestamp=%d; code=%q value=%q", timestamp, code, value)
	_, err := tx.ExecContext(ctx, `
        INSERT INTO Svalues(Timestamp, Code, Value) VALUES (?, ?, ?)
    `, timestamp, code, value)
	if err != nil {
		glog.V(1).Infof("db.AddValue: timestamp=%d; kindCode=%q value=%q: %v",
			timestamp, code, value, err)
		return fmt.Errorf("db.AddValue: could not exec tx: %w", err)
	}
	return nil
}

func FindValueById(ctx context.Context, tx *sql.Tx, id uint64) *sql.Row {
	glog.V(2).Infof("db.FindValueById: id=%v", id)
	res := tx.QueryRowContext(ctx, `
        SELECT
            Timestamp, Code, Value
        FROM
            Svalues
        WHERE
            Id = ?
        LIMIT 1;
    `, id)
	return res
}
