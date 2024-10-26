package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/filmil/go-vcd-parser/vcd"
)

func run(r io.Reader, filename string) error {

	parser := vcd.NewParser[vcd.File]()
	ast, err := parser.Parse(filename, r)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	fmt.Printf("ast: %+v", ast)
	return nil
}

func main() {
	var filename string
	flag.StringVar(&filename, "in", "", "Input filename, VCD file")
	flag.Parse()

	if filename == "" {
		log.Printf("flag --in=... is required")
		os.Exit(1)
	}

	file, err := os.Open(filename)
	if err != nil {
		log.Printf("error opening: %v: %v", filename, err)
	}

	if err := run(file, filename); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}

}
