// Package dbt adds testing primitives for populating the signal database.
package dbt

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync"

	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

var (
	counter int
	m       sync.Mutex
)

// NewMemDB returns a name for a new in-memory database.  Not thread-safe.
func NewMemDB() string {
	{
		m.Lock()
		counter++
		m.Unlock()
	}
	return fmt.Sprintf("test.%d.db?cache=shared&mode=memory", counter)
}

type Instance struct {
	db         *sql.DB
	nameToCode map[string]string
	counter    int
	ctx        context.Context
}

type Signal struct {
	parent *Instance
	code   string
	kind   vcd.VarKindCode
	size   int
	ctx    context.Context
}

func New(db *sql.DB, ctx context.Context) *Instance {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Instance{
		db:         db,
		nameToCode: map[string]string{},
		counter:    1,
		ctx:        ctx,
	}
}

type TimeValue struct {
	Time  uint64
	Value string
}

func (self *Instance) newCtx() (context.Context, func()) {
	return context.WithCancel(self.ctx)
}

func (self *Instance) newCode() string {
	ret := strconv.FormatInt(int64(self.counter), 36)
	self.counter++
	return ret
}

func (self *Instance) Signal(name string, kind vcd.VarKindCode, size int) *Signal {
	if _, ok := self.nameToCode[name]; ok {
		panic(fmt.Sprintf("Signal %q already present", name))
	}
	ctx, _ := self.newCtx()
	code := self.newCode()

	tx, err := self.db.Begin()
	if err != nil {
		panic(fmt.Sprintf("could not create transaction: %v", err))
	}
	defer func() {
		if err := tx.Commit(); err != nil {
			glog.Warningf("could not commit a signal: %v", err)
		}
	}()
	if err := db.AddSignal(ctx, tx, name, kind, code, size); err != nil {
		panic(fmt.Sprintf("could not add signal! %v", err))
	}
	self.nameToCode[name] = code
	return &Signal{
		parent: self,
		code:   code,
		kind:   kind,
		size:   size,
		ctx:    ctx,
	}
}

// TimeValues records the given time value pairs as a signal.
//
// Returns the parent `Instance` so that multiple signals could be added.
func (self *Signal) TimeValues(pairs ...TimeValue) *Instance {
	ctx, cancelFn := context.WithCancel(self.ctx)
	defer cancelFn()
	dbx := self.parent.db
	tx, err := dbx.Begin()
	if err != nil {
		panic(fmt.Sprintf("could not start transaction: %v", err))
	}
	defer func() {
		if err := tx.Commit(); err != nil {
			glog.Warningf("could not commit a signal: %v", err)
		}
	}()
	for _, p := range pairs {
		if len(p.Value) != self.size {
			panic(fmt.Sprintf("size mismatch: size=%v; pair: %+v", self.size, p))
		}
		if err := db.AddValue(ctx, tx, p.Time, self.code, p.Value); err != nil {
			panic(fmt.Sprintf("could not add value: %v", err))
		}

	}
	return self.parent
}
