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
	v map[string]string
}

var _ flag.Value = (*ArrayValue)(nil)

var separator string

func (self ArrayValue) String() string {
	ret := []string{}
	for k := range self.v {
		ret = append(ret, fmt.Sprintf("'%s'", k))
	}
	return strings.Join(ret, ",")
}

func (self *ArrayValue) Set(v string) error {
	if self.v == nil {
		self.v = map[string]string{}
	}
	s := strings.Split(v, separator)
	if len(s) == 2 {
		self.v[s[0]] = s[1]
	} else {
		self.v[v] = v
	}
	return nil
}

func (self *ArrayValue) Get(v string) string {
	if m, ok := self.v[v]; ok {
		return m
	}
	return v
}

func main() {
	var (
		inDb             string
		signals          ArrayValue
		minTime, maxTime int
		ndots            int
	)

	flag.StringVar(&inDb, "in", "", "Input sqlite signals database")
	flag.StringVar(&separator, "separator", "=>", "")
	flag.Var(&signals, "signal", "Signals to include")
	flag.IntVar(&minTime, "min-time", -1, "")
	flag.IntVar(&maxTime, "max-time", math.MaxInt, "")
	flag.IntVar(&ndots, "ndots", 1000, "")
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
        JOIN
            Signals s
        ON
            s.Code = v.Code
        WHERE
            (v.Timestamp >= ?)
                AND
            (v.Timestamp <= ?)
                AND
            s.Name IN (%v)
        ORDER BY
            v.Timestamp
        ;
    `, signals.String())

	rows, err := dbx.QueryContext(ctx, q, minTime, maxTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v", err)
		os.Exit(1)
	}

	curr := 0
	var stanzas []string
	for {
		if !rows.Next() {
			break
		}
		var (
			signal    string
			timestamp int
			value     string
		)
		err := rows.Scan(&signal, &timestamp, &value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "database error: %v", err)
			os.Exit(1)
		}
		if curr == timestamp {
			s := fmt.Sprintf("%v=%v", signals.v[signal], value)
			stanzas = append(stanzas, s)
		} else {
			// Figure out how many dots to put in.
			q := (timestamp - curr) / ndots
			fmt.Printf("%v\n", strings.Repeat(".", q))
			fmt.Printf("# timestamp: %v\n", curr)
			fmt.Printf("%v.\n", strings.Join(stanzas, ";"))

			s := fmt.Sprintf("%v=%v", signals.v[signal], value)
			stanzas = []string{s}
			curr = timestamp
		}

	}
}
