package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/filmil/go-vcd-parser/cvt"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

func main() {
	var inFile, outFile, outFmt, signalFile string
	flag.StringVar(&inFile, "in", "", "Input filename, VCD file (required)")
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

	file, err := os.Open(inFile)
	if err != nil {
		glog.Errorf("error opening: %v: %v", inFile, err)
		os.Exit(1)
	}

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
		if err := vcd.Parse(inFile, b, j); err != nil {
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
		if err := cvt.ConvertStream(ctx, inFile, b, dbx); err != nil {
			glog.Errorf("could not convert: %v", err)
			os.Exit(1)
		}
		// The indexes are built once, over the finished table, rather
		// than maintained row by row during the load.
		if err := db.FinishBulk(ctx, dbx); err != nil {
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
