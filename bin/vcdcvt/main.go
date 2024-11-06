package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/filmil/go-vcd-parser/cvt"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
	"github.com/golang/glog"
)

func run(r io.Reader, filename string) (*vcd.File, error) {

	parser := vcd.NewParser[vcd.File]()
	ast, err := parser.Parse(filename, r)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return ast, nil
}

func main() {
	var inFile, outFile, outFmt, signalFile string
	flag.StringVar(&inFile, "in", "", "Input filename, VCD file (required)")
	flag.StringVar(&outFile, "out", "", "Output filename, parsed vcd.File (required)")
	flag.StringVar(&outFmt, "format", "", "Output format to use: json, sqlite")
	flag.StringVar(&signalFile, "signals", "", "Signals CSV file to write (optional)")
	flag.IntVar(&cvt.MaxTx, "max-tx", 1000000, "Number of ops in a transaction")
	flag.Parse()

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

	start := time.Now()
	glog.Infof("parsing input from: %v", inFile)
	ast, err := run(b, inFile)
	if err != nil {
		glog.Errorf("error: %v: %v", inFile, err)
		os.Exit(1)
	}

	endLoad := time.Now()
	glog.Infof("parsing took: %v", endLoad.Sub(start))
	glog.Infof("writing output to: %v", outFile)
	startWrite := time.Now()
	if outFmt == "json" {
		of, err := os.Create(outFile)
		if err != nil {
			glog.Errorf("error: %v: %v", outFile, err)
			os.Exit(1)
		}

		e := json.NewEncoder(of)
		e.SetIndent("", "  ")
		e.SetEscapeHTML(false)
		defer of.Close()

		if err := e.Encode(ast); err != nil {
			glog.Infof("cannot encode: %v: %v", outFile, err)
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
		dbx, err := db.OpenDB(ctx, outFile)
		if err != nil {
			glog.Errorf("could not open database: %v: %v", outFile, err)
			os.Exit(1)
		}
		defer dbx.Close()
		if err := cvt.Convert(ctx, ast, dbx); err != nil {
			glog.Errorf("could not convert: %v", err)
			os.Exit(1)
		}

		if signalFile != "" {
			// Write a list of all signals.
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
