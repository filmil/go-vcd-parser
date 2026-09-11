// SPDX-License-Identifier: Apache-2.0

package fst

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/filmil/go-vcd-parser/vcd"
)

// writeFST builds a small dump with libfst's own writer, so the test reads a
// file in the real format rather than a hand-made approximation.
//
// The dump is:
//
//	$timescale 1ps
//	module top { wire clk; wire [3:0] count; module sub { wire ready } }
//	#0   clk=0 count=0000 ready=x
//	#10  clk=1 count=0001
//	#20  clk=0 ready=1
func writeFST(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.fst")

	v := func(name string, length int) wstep { return wstep{variable: &wsignal{name: name, length: length}} }
	at := func(tm uint64) wstep { return wstep{time: tm, timeValid: true} }
	set := func(i int, val string) wstep { return wstep{change: &wchange{index: i, value: val}} }
	const clkI, countI, readyI = 0, 1, 2

	// -12 is one picosecond, the exponent of seconds FST stores.
	if err := writeTestFST(path, -12, []wstep{
		{openScope: "top"},
		v("clk", 1),
		v("count [3:0]", 4),
		{openScope: "sub"},
		v("ready", 1),
		{closeScope: true},
		{closeScope: true},

		at(0), set(clkI, "0"), set(countI, "0000"), set(readyI, "x"),
		at(10), set(clkI, "1"), set(countI, "0001"),
		at(20), set(clkI, "0"), set(readyI, "1"),
	}); err != nil {
		t.Fatalf("writeTestFST: %v", err)
	}

	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		t.Fatalf("the writer produced nothing usable at %q: %v", path, err)
	}
	return path
}

// recorder is a vcd.Handler that writes every event down as text, so the
// test can compare a whole stream in one place. It copies every slice,
// which is also what makes it a check that the reader's reused buffers hold
// the right bytes at the moment of the call.
type recorder struct {
	scope []string
	ev    []string
}

func (r *recorder) Declaration(d *vcd.DeclarationCommandT) error {
	switch {
	case d.Timescale != nil:
		r.ev = append(r.ev, fmt.Sprintf("timescale %d%s",
			d.Timescale.Number, unitOf(d.Timescale.Unit)))
	case d.Scope != nil:
		r.scope = append(r.scope, d.Scope.Id)
		r.ev = append(r.ev, "scope "+strings.Join(r.scope, "/"))
	case d.Upscope != nil:
		if len(r.scope) > 0 {
			r.scope = r.scope[:len(r.scope)-1]
		}
		r.ev = append(r.ev, "upscope")
	case d.Var != nil:
		v := d.Var
		r.ev = append(r.ev, fmt.Sprintf("var %s %d %s %s",
			v.VarType, v.Size, v.Code, v.Id.String()))
	case d.EndDefinitions != nil:
		r.ev = append(r.ev, "enddefinitions")
	}
	return nil
}

func (r *recorder) Timestamp(ts uint64, raw []byte) error {
	r.ev = append(r.ev, fmt.Sprintf("time %d raw=%s", ts, string(raw)))
	return nil
}

func (r *recorder) ValueChange(k vcd.ValueKind, value, idcode []byte) error {
	r.ev = append(r.ev, fmt.Sprintf("value %s = %s", string(idcode), string(k.Payload(value))))
	return nil
}

func (r *recorder) DumpBegin(vcd.DumpKind) error   { return nil }
func (r *recorder) DumpEnd(vcd.DumpKind) error     { return nil }
func (r *recorder) Directive(string, []byte) error { return nil }

func unitOf(u *vcd.TimeUnit) string {
	switch {
	case u == nil:
		return "?"
	case u.Second:
		return "s"
	case u.MilliSecond:
		return "ms"
	case u.MicroSecond:
		return "us"
	case u.NanoSecond:
		return "ns"
	case u.PicoSecond:
		return "ps"
	case u.FemtoSecond:
		return "fs"
	}
	return "?"
}

func TestParseRoundTrip(t *testing.T) {
	path := writeFST(t)

	var r recorder
	if err := Parse(path, &r); err != nil {
		t.Fatalf("Parse(%q): %v", path, err)
	}

	// The identifier codes come from the FST handles, which the writer
	// hands out from 1, so they are the first three of the alphabet.
	clk, count, ready := idCode(1), idCode(2), idCode(3)

	want := []string{
		"timescale 1ps",
		"scope top",
		"var wire 1 " + clk + " clk",
		"var wire 4 " + count + " count[3:0]",
		"scope top/sub",
		"var wire 1 " + ready + " ready",
		"upscope",
		"upscope",
		"enddefinitions",
		"time 0 raw=0",
		"value " + clk + " = 0",
		"value " + count + " = 0000",
		"value " + ready + " = x",
		"time 10 raw=10",
		"value " + clk + " = 1",
		"value " + count + " = 0001",
		"time 20 raw=20",
		"value " + clk + " = 0",
		"value " + ready + " = 1",
	}

	got := sortWithinTimestamp(r.ev)
	want = sortWithinTimestamp(want)
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d:\n got: %s\nwant: %s", i, got[i], want[i])
		}
	}
}

// sortWithinTimestamp sorts each run of value changes.
//
// The changes at one timestamp happen at the same instant, and libfst hands
// them over in the order its block holds them, which is not the order the
// writer emitted them in. Ordering between timestamps is meaningful and is
// left alone; ordering inside one is not, and asserting it would make the
// test fail on a detail of the format that carries no information.
func sortWithinTimestamp(ev []string) []string {
	out := append([]string(nil), ev...)
	run := -1
	flush := func(end int) {
		if run >= 0 && end-run > 1 {
			sort.Strings(out[run:end])
		}
		run = -1
	}
	for i, e := range out {
		if strings.HasPrefix(e, "value ") {
			if run < 0 {
				run = i
			}
			continue
		}
		flush(i)
	}
	flush(len(out))
	return out
}

// TestParseNotAnFST checks the error path rather than leaving a corrupt file
// to be reported as an empty dump.
func TestParseNotAnFST(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not.fst")
	if err := os.WriteFile(path, []byte("$date\n  today\n$end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Parse(path, &recorder{}); err == nil {
		t.Error("Parse of a VCD file named .fst returned no error")
	}
}

func TestParseMissingFile(t *testing.T) {
	if err := Parse(filepath.Join(t.TempDir(), "absent.fst"), &recorder{}); err == nil {
		t.Error("Parse of a file that does not exist returned no error")
	}
}

// errHandler fails on the first value change, to check that the error
// reaches the caller rather than being swallowed in the C callback.
type errHandler struct {
	recorder
	err error
}

func (e *errHandler) ValueChange(vcd.ValueKind, []byte, []byte) error { return e.err }

func TestParseHandlerError(t *testing.T) {
	path := writeFST(t)
	want := fmt.Errorf("stop here")
	err := Parse(path, &errHandler{err: want})
	if err == nil {
		t.Fatal("Parse returned no error when the handler failed")
	}
	if !strings.Contains(err.Error(), "stop here") {
		t.Errorf("Parse error = %v, want it to name the handler's error", err)
	}
}

// TestParseReal covers a real-valued signal.
//
// libfst formats a real with "%.16g" rather than handing over the eight
// bytes of the double, so the value's text is as long as the number needs.
// Trusting the declared length, which is 8 for every real, truncates a long
// value and reads past the end of a short one.
func TestParseReal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real.fst")

	at := func(tm uint64) wstep { return wstep{time: tm, timeValid: true} }
	setR := func(i int, v float64) wstep { return wstep{change: &wchange{index: i, realValue: v}} }

	if err := writeTestFST(path, -12, []wstep{
		{openScope: "top"},
		{variable: &wsignal{name: "v", length: 8, real: true}},
		{closeScope: true},

		// Short text, well under the eight bytes of the double.
		at(0), setR(0, 1.5),
		// Text far longer than eight bytes.
		at(10), setR(0, 1.0/3.0),
		at(20), setR(0, 0),
	}); err != nil {
		t.Fatalf("writeTestFST: %v", err)
	}

	var r recorder
	if err := Parse(path, &r); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	code := idCode(1)
	want := []string{
		"timescale 1ps",
		"scope top",
		"var real 8 " + code + " v",
		"upscope",
		"enddefinitions",
		"time 0 raw=0",
		"value " + code + " = 1.5",
		"time 10 raw=10",
		"value " + code + " = 0.3333333333333333",
		"time 20 raw=20",
		"value " + code + " = 0",
	}
	if len(r.ev) != len(want) {
		t.Fatalf("got %d events, want %d\n got: %v\nwant: %v", len(r.ev), len(want), r.ev, want)
	}
	for i := range want {
		if r.ev[i] != want[i] {
			t.Errorf("event %d:\n got: %q\nwant: %q", i, r.ev[i], want[i])
		}
	}
}
