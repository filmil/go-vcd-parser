package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/filmil/go-vcd-parser/vcd"
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
	var inFile, outFile string
	flag.StringVar(&inFile, "in", "", "Input filename, VCD file")
	flag.StringVar(&outFile, "out", "", "Output filename, parsed vcd.File")
	flag.Parse()

	if inFile == "" {
		log.Printf("flag --in=... is required")
		os.Exit(1)
	}

	file, err := os.Open(inFile)
	if err != nil {
		log.Printf("error opening: %v: %v", inFile, err)
	}

	ast, err := run(file, inFile)
	if err != nil {
		log.Printf("error: %v: %v", inFile, err)
		os.Exit(1)
	}

	of, err := os.Create(outFile)
	if err != nil {
		log.Printf("error: %v: %v", outFile, err)
	}

	e := json.NewEncoder(of)
	defer of.Close()

	if err := e.Encode(ast); err != nil {
		log.Printf("cannot encode: %v: %v", outFile, err)
		os.Exit(1)
	}
}
