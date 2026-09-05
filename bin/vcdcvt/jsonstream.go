package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/filmil/go-vcd-parser/vcd"
)

// jsonWriter streams a vcd.File as JSON without ever holding the whole
// tree. The bytes it produces are identical to what
//
//	e := json.NewEncoder(w); e.SetIndent("", "  "); e.SetEscapeHTML(false)
//	e.Encode(file)
//
// writes for the same file, including the `omitempty` handling that drops
// an empty section entirely rather than printing `[]`.
type jsonWriter struct {
	w   io.Writer
	err error

	// open is the section currently being written, "" before the first
	// element. Sections are written in File's field order, and a section
	// only starts once it has an element.
	open  string
	first bool

	// ast rebuilds the nodes to marshal. The value change section is
	// rendered element by element, so only one is ever held.
	ast vcd.NodeBuilder
}

func newJSONWriter(w io.Writer) *jsonWriter { return &jsonWriter{w: w} }

func (j *jsonWriter) write(s string) {
	if j.err != nil {
		return
	}
	_, j.err = io.WriteString(j.w, s)
}

// element marshals v the way json.Encoder would at nesting depth two.
func (j *jsonWriter) element(section string, v any) error {
	if j.err != nil {
		return j.err
	}
	if j.open != section {
		if j.open == "" {
			j.write("{\n")
		} else {
			j.write("\n  ],\n")
		}
		j.write(fmt.Sprintf("  %q: [\n", section))
		j.open, j.first = section, true
	}
	if !j.first {
		j.write(",\n")
	}
	j.first = false

	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	e.SetIndent("    ", "  ")
	if err := e.Encode(v); err != nil {
		return fmt.Errorf("could not encode: %w", err)
	}
	j.write("    ")
	j.write(string(bytes.TrimRight(buf.Bytes(), "\n")))
	return j.err
}

// Close finishes the document.
func (j *jsonWriter) Close() error {
	if j.err != nil {
		return j.err
	}
	if j.open == "" {
		// No elements at all: File marshals to an empty object.
		j.write("{}\n")
		return j.err
	}
	j.write("\n  ]\n}\n")
	return j.err
}

func (j *jsonWriter) Declaration(d *vcd.DeclarationCommandT) error {
	return j.element("DeclarationCommand", d)
}

func (j *jsonWriter) sim(s *vcd.SimulationCommandT) error {
	return j.element("SimulationCommand", s)
}

func (j *jsonWriter) Timestamp(ts uint64, raw []byte) error {
	return j.sim(j.ast.Timestamp(ts, raw))
}

func (j *jsonWriter) ValueChange(k vcd.ValueKind, value, idcode []byte) error {
	if s := j.ast.ValueChange(k, value, idcode); s != nil {
		return j.sim(s)
	}
	return nil // buffered into the open dump block
}

func (j *jsonWriter) SetPos(p lexer.Position) { j.ast.SetPos(p) }

func (j *jsonWriter) DumpBegin(k vcd.DumpKind) error { return j.ast.DumpBegin(k) }

func (j *jsonWriter) DumpEnd(k vcd.DumpKind) error { return j.sim(j.ast.DumpEnd(k)) }

func (j *jsonWriter) Directive(name string, text []byte) error {
	if s := j.ast.Directive(name, text); s != nil {
		return j.sim(s)
	}
	return nil
}
