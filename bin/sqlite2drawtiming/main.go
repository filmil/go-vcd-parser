package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/filmil/go-vcd-parser/db"
)

type ArrayValue struct {
	v []string
}

var _ flag.Value = (*ArrayValue)(nil)

func (self ArrayValue) String() string {
	ret := []string{}
	for _, s := range self.v {
		ret = append(ret, fmt.Sprintf("'%s'", s))
	}
	return strings.Join(ret, ",")
}

func (self *ArrayValue) Set(v string) error {
	self.v = append(self.v, v)
	return nil
}

func main() {
	var (
		inDb             string
		signals          ArrayValue
		minTime, maxTime int
	)

	flag.StringVar(&inDb, "in", "", "Input sqlite signals database")
	flag.Var(&signals, "signal", "Signals to include")
	flag.IntVar(&minTime, "min-time", 0, "")
	flag.IntVar(&maxTime, "max-time", math.MaxInt, "")
	flag.Parse()

	if inDb == "" {
		fmt.Fprintf(os.Stderr, "flag --in=... is required")
		os.Exit(1)
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithCancel(ctx)
	defer cancelFn()

	dbx, err := db.OpenDB(ctx, inDb)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not open input database: %v: %v", inDb, err)
	}

	if len(signals.v) == 0 {
		fmt.Fprintf(os.Stderr, "at least one --signal=... is required")
		os.Exit(1)
	}

	q := fmt.Sprintf(`
        SELECT
            s.Name,
            v.Timestamp,
            v.Value
        FROM
            Svalues v
        LEFT JOIN
            Signals s
        ON
            s.Code = v.Code

        WHERE
            s.Name IN (%v)
        ORDER BY
            v.Timestamp
        ;
    `, signals.String())

	fmt.Printf("%v", q)
	rows, err := dbx.QueryContext(ctx, q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v", err)
		os.Exit(1)
	}
	if rows.Next() {
		var (
			signal    string
			timestamp uint
			value     string
		)
		err := rows.Scan(&signal, &timestamp, &value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "database error: %v", err)
			os.Exit(1)
		}
		fmt.Printf("%v %v %v", signal, timestamp, value)
	} else {
		fmt.Printf("no rows?\n")
	}

}
