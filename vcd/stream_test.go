package vcd

import (
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// eventLog records the event stream as readable strings.
type eventLog struct {
	NopHandler
	got []string
}

func (e *eventLog) Timestamp(ts uint64, _ []byte) error {
	e.got = append(e.got, fmt.Sprintf("#%d", ts))
	return nil
}

func (e *eventLog) ValueChange(k ValueKind, value, idcode []byte) error {
	e.got = append(e.got, fmt.Sprintf("%v %q %q", k, value, idcode))
	return nil
}

func (e *eventLog) DumpBegin(k DumpKind) error {
	e.got = append(e.got, "begin "+k.String())
	return nil
}

func (e *eventLog) DumpEnd(k DumpKind) error {
	e.got = append(e.got, "end "+k.String())
	return nil
}

func (e *eventLog) Directive(name string, text []byte) error {
	e.got = append(e.got, fmt.Sprintf("%v %q", name, text))
	return nil
}

func (k ValueKind) String() string {
	switch k {
	case ValueScalar:
		return "scalar"
	case ValueBinary:
		return "binary"
	case ValueReal:
		return "real"
	case ValueString:
		return "string"
	}
	return "?"
}

// TestBodyDispatch pins how each shape of word in the value change section
// is decoded.
func TestBodyDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		body string
		want []string
	}{
		// A scalar value is glued to its code.
		{`0!`, []string{`scalar "0" "!"`}},
		{`x*@`, []string{`scalar "x" "*@"`}},
		{`0V#`, []string{`scalar "0" "V#"`}},
		{`0###VAB`, []string{`scalar "0" "###VAB"`}},
		// std_logic values the grammar's ValueT does not accept.
		{`U!`, []string{`scalar "U" "!"`}},
		{`-!`, []string{`scalar "-" "!"`}},
		// A lone value character takes the next word as its code...
		{`0 !`, []string{`scalar "0" "!"`}},
		// ...unless that word starts a command of its own.
		{"0\n#5", []string{`scalar "0" ""`, "#5"}},
		// Vectors take the next word verbatim, whatever it looks like.
		{`b1010 %`, []string{`binary "b1010" "%"`}},
		{`b0 #0`, []string{`binary "b0" "#0"`}},
		{`b0 R0`, []string{`binary "b0" "R0"`}},
		{`b0 b0`, []string{`binary "b0" "b0"`}},
		{`b0 $end`, []string{`binary "b0" "$end"`}},
		// A vector glued to its code splits at the first character
		// that cannot be part of the value.
		{`bx(k`, []string{`binary "bx" "(k"`}},
		{`b00000000 9#`, []string{`binary "b00000000" "9#"`}},
		{`r3.14 x`, []string{`real "r3.14" "x"`}},
		{`sIDLE ^`, []string{`string "sIDLE" "^"`}},
		// A string value is never split: it may hold punctuation.
		{`sfoo-bar X`, []string{`string "sfoo-bar" "X"`}},
		{`#500`, []string{"#500"}},
		{`#0`, []string{"#0"}},
		{
			"$dumpvars\nx*@\nb0 (k\n$end",
			[]string{"begin $dumpvars", `scalar "x" "*@"`, `binary "b0" "(k"`, "end $dumpvars"},
		},
		{`$dumpall $end`, []string{"begin $dumpall", "end $dumpall"}},
		// A comment in the body is consumed whole, so the words in it
		// -- including $dumpvars and #500 -- are not commands. It has
		// to follow a value change to be in the body at all: a comment
		// straight after $enddefinitions is still a declaration, which
		// is what the grammar does with it too.
		{
			"1!\n$comment $dumpvars at #500 $end\n0!",
			[]string{`scalar "1" "!"`, `$comment "$dumpvars at #500"`, `scalar "0" "!"`},
		},
	}
	for _, test := range tests {
		t.Run(test.body, func(t *testing.T) {
			var log eventLog
			input := "$enddefinitions $end\n" + test.body + "\n"
			if err := Parse("(test)", strings.NewReader(input), &log); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got := fmt.Sprint(log.got); got != fmt.Sprint(test.want) {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

// TestBodyErrors covers input the streaming parser must reject rather than
// silently misread.
func TestBodyErrors(t *testing.T) {
	t.Parallel()
	tests := []string{
		`$end`,           // no dump block is open
		`$dumpvars 1!`,   // unterminated
		`#`,              // timestamp with no digits
		`#12x`,           // not a number
		`qq`,             // not a value
		`$dumpvars $dumpall $end`, // nested dump blocks
	}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			input := "$enddefinitions $end\n" + test + "\n"
			if err := Parse("(test)", strings.NewReader(input), &NopHandler{}); err == nil {
				t.Errorf("expected an error for %q", test)
			}
		})
	}
}

// bodyReader synthesises a value change section of n changes without ever
// holding it in memory, so that a parse of any size can be measured.
type bodyReader struct {
	n, i int
	line []byte
	off  int
}

func newBodyReader(n int) *bodyReader {
	return &bodyReader{n: n, line: []byte("$enddefinitions $end\n")}
}

// next builds the following line into the reader's own buffer. It must not
// allocate, or it would swamp the allocation measurement it exists to
// support.
func (b *bodyReader) next() bool {
	if b.i > b.n {
		return false
	}
	b.line, b.off = b.line[:0], 0
	if b.i%2 == 0 {
		b.line = append(b.line, '#')
		b.line = strconv.AppendUint(b.line, uint64(b.i), 10)
		b.line = append(b.line, '\n')
	} else {
		b.line = append(b.line, 'b')
		b.line = strconv.AppendUint(b.line, uint64(b.i), 2)
		b.line = append(b.line, ' ', '!', '\n', '0', '"', '\n')
	}
	b.i++
	return true
}

func (b *bodyReader) Read(p []byte) (int, error) {
	total := 0
	for total < len(p) {
		if b.off >= len(b.line) && !b.next() {
			if total == 0 {
				return 0, io.EOF
			}
			return total, nil
		}
		n := copy(p[total:], b.line[b.off:])
		b.off += n
		total += n
	}
	return total, nil
}

// TestParseAllocationsAreConstant is the real proof that the parser
// streams: a hundredfold longer input must not cost more allocations.
func TestParseAllocationsAreConstant(t *testing.T) {
	parse := func(n int) func() {
		return func() {
			if err := Parse("(bench)", newBodyReader(n), &NopHandler{}); err != nil {
				t.Fatalf("parse error: %v", err)
			}
		}
	}
	small := testing.AllocsPerRun(5, parse(1_000))
	large := testing.AllocsPerRun(5, parse(100_000))
	if grew := large - small; grew > 8 {
		t.Errorf("allocations grew by %v between a 1e3 and a 1e5 change input; "+
			"parsing is retaining something (small=%v large=%v)", grew, small, large)
	}
}

// TestParseHeapIsBounded parses far more input than would fit in the
// ceiling it asserts. The grammar-driven parser needs roughly the size of
// the file plus its token stream, so it could not pass this.
func TestParseHeapIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large input test in short mode")
	}
	const changes = 4_000_000 // several hundred MB of input
	var counter countingHandler
	if err := Parse("(big)", newBodyReader(changes), &counter); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if counter.changes == 0 {
		t.Fatal("no value changes seen")
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const ceiling = 32 << 20
	if m.HeapAlloc > ceiling {
		t.Errorf("heap after parsing %v changes is %v bytes, want under %v",
			counter.changes, m.HeapAlloc, ceiling)
	}
	t.Logf("parsed %v value changes, heap %v bytes", counter.changes, m.HeapAlloc)
}

type countingHandler struct {
	NopHandler
	changes int
}

func (c *countingHandler) ValueChange(ValueKind, []byte, []byte) error {
	c.changes++
	return nil
}
