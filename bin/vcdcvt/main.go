package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/filmil/go-vcd-parser/cvt"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/fst"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

func main() {
	var inFile, outFile, outFmt, inFmt, signalFile string
	flag.StringVar(&inFile, "in", "", "Input filename, a VCD or FST dump (required)")
	flag.StringVar(&inFmt, "in-format", "auto",
		"Input format: auto, vcd, fst. auto reads a .fst file as FST and anything else as VCD")
	flag.StringVar(&outFile, "out", "", "Output filename, parsed vcd.File (required)")
	flag.StringVar(&outFmt, "format", "", "Output format to use: json, sqlite")
	flag.StringVar(&signalFile, "signals", "", "Signals CSV file to write (optional)")
	flag.IntVar(&cvt.MaxTx, "max-tx", 1000000, "Number of ops in a transaction")
	flag.Parse()

	pwd, _ := os.Getwd()
	glog.Infof("PWD: %v", pwd)

	if inFile == "" {
		glog.Errorf("flag --in=... is required")
		os.Exit(1)
	}
	if outFile == "" {
		glog.Errorf("flag --out=... is required")
		os.Exit(1)
	}
	if (outFmt != "json") && (outFmt != "sqlite") {
		glog.Errorf("flag --format=json|sqlite is required")
		os.Exit(1)
	}
	isFST, err := readFST(inFmt, inFile)
	if err != nil {
		glog.Errorf("%v", err)
		os.Exit(1)
	}

	// libfst opens the dump by name, because FST reaches its value change
	// blocks through a table at the end of the file. Only the VCD path
	// reads the handle opened here.
	file, err := os.Open(inFile)
	if err != nil {
		glog.Errorf("error opening: %v: %v", inFile, err)
		os.Exit(1)
	}
	defer file.Close()

	b := bufio.NewReaderSize(file, 1000000)

	// Both output formats consume the file as it is read, so memory use
	// does not grow with the size of the dump.
	glog.Infof("converting %v to %v", inFile, outFile)
	startWrite := time.Now()
	if outFmt == "json" {
		of, err := os.Create(outFile)
		if err != nil {
			glog.Errorf("error: %v: %v", outFile, err)
			os.Exit(1)
		}
		defer of.Close()
		w := bufio.NewWriterSize(of, 1000000)
		j := newJSONWriter(w)
		// Both readers report the same events, so one writer serves both.
		parse := func() error { return vcd.Parse(inFile, b, j) }
		if isFST {
			parse = func() error { return fst.Parse(inFile, j) }
		}
		if err := parse(); err != nil {
			of.Close()
			os.Remove(outFile)
			glog.Errorf("error: %v: %v", inFile, err)
			os.Exit(1)
		}
		if err := j.Close(); err != nil {
			glog.Errorf("cannot encode: %v: %v", outFile, err)
			os.Exit(1)
		}
		if err := w.Flush(); err != nil {
			glog.Errorf("cannot write: %v: %v", outFile, err)
			os.Exit(1)
		}
	}

	if outFmt == "sqlite" {
		_, err := os.Stat(outFile)
		if err == nil || os.IsExist(err) {
			glog.V(2).Infof("clearing file: %v", outFile)
			if err := os.Remove(outFile); err != nil {
				glog.Errorf("could not remove: %v: %v", outFile, err)
				os.Exit(1)
			}
		} else if !os.IsNotExist(err) {
			glog.Errorf("could not stat: %v: %v", outFile, err)
			os.Exit(1)
		}
		ctx := context.Background()
		dbx, err := db.OpenBulk(ctx, outFile)
		if err != nil {
			glog.Errorf("could not open database: %v: %v", outFile, err)
			os.Exit(1)
		}
		defer dbx.Close()
		convert := func() error { return cvt.ConvertStream(ctx, inFile, b, dbx) }
		if isFST {
			convert = func() error { return cvt.ConvertFST(ctx, inFile, dbx) }
		}
		if err := convert(); err != nil {
			discard(dbx, outFile)
			glog.Errorf("could not convert: %v", err)
			os.Exit(1)
		}
		// The indexes are built once, over the finished table, rather
		// than maintained row by row during the load.
		if err := db.FinishBulk(ctx, dbx); err != nil {
			discard(dbx, outFile)
			glog.Errorf("could not finish the load: %v", err)
			os.Exit(1)
		}

		if signalFile != "" {
			glog.Infof("writing signals dump file: %q", signalFile)
			of, err := os.Create(signalFile)
			if err != nil {
				glog.Warningf("did not write a signals file: %v", err)
			} else {
				defer of.Close()
				w := csv.NewWriter(of)
				rows, err := dbx.Query(`SELECT Name, Type, Size FROM Signals;`)
				if err != nil {
					glog.Warningf("could not execute query: %v", err)
					os.Exit(1)
				}
				if err := w.Write([]string{"name", "type", "size"}); err != nil {
					glog.Warningf("could not write: %v", err)
					os.Exit(1)
				}
				for rows.Next() {
					n, t, s, err := db.Scan3NoNext[string, int, int](rows)
					if err != nil {
						glog.Warningf("could not scan: %v", err)
						os.Exit(1)
					}
					glog.Infof("%v %v %v", *n, *t, *s)
					ts, ss := strconv.Itoa(*t), strconv.Itoa(*s)
					if err := w.Write([]string{*n, ts, ss}); err != nil {
						glog.Warningf("could not write: %v", err)
						os.Exit(1)
					}
				}
			}
		}
	}
	endWrite := time.Now()
	glog.Infof("Done. Writing took: %v", endWrite.Sub(startWrite))

}

// readFST decides whether the input is an FST dump.
//
// `auto` goes by the file name, which is how the dump writers name their
// output, and is what -in-format overrides when a dump is named otherwise.
func readFST(inFmt, inFile string) (bool, error) {
	switch inFmt {
	case "fst":
		return true, nil
	case "vcd":
		return false, nil
	case "auto":
		return strings.EqualFold(filepath.Ext(inFile), ".fst"), nil
	}
	return false, fmt.Errorf("flag --in-format=auto|vcd|fst, got: %q", inFmt)
}

// discard closes the database and removes it.
//
// A conversion that fails has written a file that is not a conversion of
// anything. Leaving it behind makes a failed run look like one that produced
// an empty result, and the next command in a script reads it as if it were
// one. os.Exit does not run deferred closes, so this closes explicitly.
func discard(dbx *sql.DB, path string) {
	if err := dbx.Close(); err != nil {
		glog.Warningf("could not close %v: %v", path, err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		glog.Warningf("could not remove the unfinished %v: %v", path, err)
	}
}
