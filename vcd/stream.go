package vcd

import (
	"fmt"
	"io"
	"sync"

	participle "github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// DumpKind identifies which of the four dump blocks is being reported.
type DumpKind int

const (
	DumpAll DumpKind = iota
	DumpOff
	DumpOn
	DumpVars
)

func (k DumpKind) String() string {
	switch k {
	case DumpAll:
		return "$dumpall"
	case DumpOff:
		return "$dumpoff"
	case DumpOn:
		return "$dumpon"
	case DumpVars:
		return "$dumpvars"
	}
	return fmt.Sprintf("DumpKind(%d)", int(k))
}

// ValueKind tells apart the four shapes a value change can take.
type ValueKind int

const (
	// ValueScalar is a one-character value glued to its identifier code,
	// as in `0!` or `x*@`.
	ValueScalar ValueKind = iota
	// ValueBinary is a `b`-prefixed vector, as in `b1010 %`.
	ValueBinary
	// ValueReal is an `r`-prefixed real, as in `r3.14 x`.
	ValueReal
	// ValueString is an `s`-prefixed string, as in `sIDLE ^`.
	ValueString
)

// Payload strips the type prefix from a raw value, returning what the
// corresponding AST node's GetValue reports.
func (k ValueKind) Payload(value []byte) []byte {
	if k == ValueScalar || len(value) == 0 {
		return value
	}
	return value[1:]
}

// Handler consumes a VCD file as it is read. Any method returning an error
// aborts the read, and that error is returned from Parse.
//
// The []byte arguments alias the reader's internal buffer and are only valid
// for the duration of the call. Wrap a Handler in CopyHandler, or copy what
// you need, rather than retaining them.
type Handler interface {
	// Declaration reports one command from the header. The header is
	// bounded by the number of signals, so this keeps the AST shape.
	Declaration(*DeclarationCommandT) error
	// Timestamp reports a `#123` simulation time. raw is the original
	// token, which the AST keeps verbatim; ts is it already parsed.
	Timestamp(ts uint64, raw []byte) error
	// ValueChange reports one value change. value is the raw token,
	// including any type prefix; use kind.Payload to strip it.
	ValueChange(kind ValueKind, value, idcode []byte) error
	// DumpBegin and DumpEnd bracket a `$dumpvars`-style block.
	DumpBegin(DumpKind) error
	DumpEnd(DumpKind) error
	// Directive reports a `$name ... $end` block in the value change
	// section: $comment, $attrbegin, $attrend, or a writer extension.
	// text is the block body, with words separated by single spaces.
	Directive(name string, text []byte) error
}

// PosSink is an optional extra interface on a Handler. When a Handler
// implements it, the parser reports the source position of each scalar
// value change just before reporting the change itself.
//
// ScalarValueChangeT is the only grammar node that records a position, so
// this only has to be implemented to reproduce that field exactly. Keeping
// it out of Handler leaves the common case -- writing rows to a database --
// free of the cost.
type PosSink interface {
	SetPos(lexer.Position)
}

// NopHandler discards everything. Embed it to implement only the methods
// you care about.
type NopHandler struct{}

func (NopHandler) Declaration(*DeclarationCommandT) error         { return nil }
func (NopHandler) Timestamp(uint64, []byte) error                 { return nil }
func (NopHandler) ValueChange(ValueKind, []byte, []byte) error    { return nil }
func (NopHandler) DumpBegin(DumpKind) error                       { return nil }
func (NopHandler) DumpEnd(DumpKind) error                         { return nil }
func (NopHandler) Directive(string, []byte) error                 { return nil }

// StringHandler is the string-typed counterpart of Handler, for callers that
// would otherwise have to remember not to retain a []byte.
type StringHandler interface {
	Declaration(*DeclarationCommandT) error
	Timestamp(ts uint64, raw string) error
	ValueChange(kind ValueKind, value, idcode string) error
	DumpBegin(DumpKind) error
	DumpEnd(DumpKind) error
	Directive(name, text string) error
}

// CopyHandler adapts a StringHandler to a Handler, copying every slice it is
// handed. Safe to retain, at the cost of an allocation per event.
type CopyHandler struct{ H StringHandler }

func (c CopyHandler) Declaration(d *DeclarationCommandT) error { return c.H.Declaration(d) }
func (c CopyHandler) Timestamp(ts uint64, raw []byte) error    { return c.H.Timestamp(ts, string(raw)) }
func (c CopyHandler) DumpBegin(k DumpKind) error               { return c.H.DumpBegin(k) }
func (c CopyHandler) DumpEnd(k DumpKind) error                 { return c.H.DumpEnd(k) }
func (c CopyHandler) ValueChange(k ValueKind, v, id []byte) error {
	return c.H.ValueChange(k, string(v), string(id))
}
func (c CopyHandler) Directive(name string, text []byte) error {
	return c.H.Directive(name, string(text))
}

// declarationKeywords are the words a DeclarationCommandT can start with.
// The header ends at the first top-level word that is not one of these,
// which is how participle's greedy `DeclarationCommand @@*` behaves --
// note that this is *not* the same as stopping at `$enddefinitions`.
var declarationKeywords = map[string]bool{
	"$comment":        true,
	"$date":           true,
	"$version":        true,
	"$var":            true,
	"$attrbegin":      true,
	"$attrend":        true,
	"$scope":          true,
	"$upscope":        true,
	"$timescale":      true,
	"$enddefinitions": true,
}

var dumpKeywords = map[string]DumpKind{
	"$dumpall":  DumpAll,
	"$dumpoff":  DumpOff,
	"$dumpon":   DumpOn,
	"$dumpvars": DumpVars,
}

// binaryValueChars and realValueChars are the characters a `b`- or
// `r`-prefixed value may contain. They decide where a glued value ends and
// its identifier code begins, mirroring the prefix-anchored regex lexer:
// BinstringPattern matches `bx` out of `bx(k`, leaving `(k` as the code.
//
// The binary set is wider than BinstringPattern's `[10xXzZuU]`: it also
// takes the remaining std_logic characters, which the regex lexer silently
// mis-splits today.
const (
	binaryValueChars = "01xXzZuUwWlLhH-"
	realValueChars   = "0123456789.+-eE"
	scalarValueChars = "01xXzZuUwWlLhH-"
)

// parseUint reads decimal digits without allocating the string that
// strconv.ParseUint would need. Timestamps are the most common word in a
// large dump, so this runs millions of times.
func parseUint(p []byte) (uint64, error) {
	if len(p) == 0 {
		return 0, fmt.Errorf("no digits")
	}
	var n uint64
	for _, c := range p {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a decimal digit: %q", c)
		}
		d := uint64(c - '0')
		if n > (1<<64-1-d)/10 {
			return 0, fmt.Errorf("value out of range")
		}
		n = n*10 + d
	}
	return n, nil
}

func inSet(set string, c byte) bool {
	for i := 0; i < len(set); i++ {
		if set[i] == c {
			return true
		}
	}
	return false
}

// headerChunkSize is how much header text is handed to participle at once.
// Chunks always end on a declaration block boundary, so the lexer starts
// each one in its Root state.
const headerChunkSize = 1 << 20

// maxDeclarationBlock caps a single `$... $end` block, which is otherwise
// unbounded -- a $comment can hold anything.
const maxDeclarationBlock = 16 << 20

// declListT is the header-only counterpart of File. Parsing the header in
// chunks with this grammar reuses every declaration rule -- $var identifier
// indices, $scope kinds, $timescale units -- with no duplication.
type declListT struct {
	Cmds []*DeclarationCommandT `parser:"@@*"`
}

var (
	declParserOnce sync.Once
	declParser     *participle.Parser[declListT]
)

func getDeclParser() *participle.Parser[declListT] {
	declParserOnce.Do(func() { declParser = NewParser[declListT]() })
	return declParser
}

// reader carries the state of one streaming parse.
type reader struct {
	sc *wordScanner
	h  Handler

	// value holds a value whose identifier code is the next word. It has
	// to be copied out because the scanner buffer is reused.
	value []byte
	// text accumulates the body of a $... $end directive.
	text []byte

	// pending holds a word that was read and put back, and one is a
	// scratch buffer for a single-character scalar value.
	pending    []byte
	hasPending bool
	one        [1]byte

	// wordPos is the position of the word nextWord last returned, which
	// pendingPos preserves across a pushBack.
	wordPos, pendingPos lexer.Position

	pos     PosSink
	wantPos bool
}

// nextWord returns the next word, honouring a pushBack.
func (rd *reader) nextWord() ([]byte, bool) {
	if rd.hasPending {
		rd.hasPending = false
		rd.wordPos = rd.pendingPos
		return rd.pending, true
	}
	w, ok := rd.sc.Next()
	rd.wordPos = rd.sc.WordPos()
	return w, ok
}

// pushBack returns a word to the stream. It copies, since the scanner
// buffer is reused.
func (rd *reader) pushBack(w []byte) {
	rd.pending = append(rd.pending[:0], w...)
	rd.pendingPos = rd.wordPos
	rd.hasPending = true
}

// Parse reads a VCD file from r, reporting each construct to h as it is
// read. Memory use is proportional to the largest declaration block and the
// widest dump block, not to the size of the file.
func Parse(filename string, r io.Reader, h Handler) error {
	rd := &reader{sc: newWordScanner(filename, r), h: h}
	rd.pos, rd.wantPos = h.(PosSink)
	return rd.run()
}

func (rd *reader) run() error {
	pending, err := rd.header()
	if err != nil {
		return err
	}
	if pending != nil {
		if err := rd.body(pending); err != nil {
			return err
		}
	}
	return rd.sc.Err()
}

// errorf reports a parse error at the current word's position.
func (rd *reader) errorf(format string, args ...any) error {
	return fmt.Errorf("%v: %s", rd.wordPos, fmt.Sprintf(format, args...))
}

// header consumes declaration blocks until a word that cannot start one.
// It returns that word, which belongs to the value change section, or nil
// at end of input.
func (rd *reader) header() ([]byte, error) {
	rd.sc.recording = true
	defer func() { rd.sc.recording = false }()
	for {
		w, ok := rd.nextWord()
		if !ok {
			return nil, rd.flushChunk()
		}
		if !declarationKeywords[string(w)] {
			rd.sc.dropLast(len(w))
			if err := rd.flushChunk(); err != nil {
				return nil, err
			}
			return w, nil
		}
		// Everything recorded before this word ends on a block
		// boundary, trailing whitespace included, so it can be handed
		// to participle now without changing what the grammar sees.
		if len(rd.sc.rec)-len(w) >= headerChunkSize {
			rd.sc.dropLast(len(w))
			if err := rd.flushChunk(); err != nil {
				return nil, err
			}
			rd.sc.rec = append(rd.sc.rec, w...)
		}
		blockStart := len(rd.sc.rec) - len(w)
		// Take the rest of the block. Blocks do not nest: only a
		// standalone `$end` closes one, so `$comment ... $enddefinitions
		// ... $end` stays a single comment.
		for {
			w, ok := rd.nextWord()
			if !ok {
				return nil, rd.flushChunk()
			}
			if string(w) == "$end" {
				break
			}
			if len(rd.sc.rec)-blockStart > maxDeclarationBlock {
				return nil, rd.errorf("declaration block exceeds %d bytes",
					maxDeclarationBlock)
			}
		}
	}
}

// flushChunk parses the recorded declaration text and reports each command.
func (rd *reader) flushChunk() error {
	if len(rd.sc.rec) == 0 {
		return nil
	}
	list, err := getDeclParser().ParseBytes(rd.sc.filename, rd.sc.rec)
	if err != nil {
		return fmt.Errorf("could not parse declarations: %w", err)
	}
	rd.sc.rec = rd.sc.rec[:0]
	for _, c := range list.Cmds {
		if err := rd.h.Declaration(c); err != nil {
			return err
		}
	}
	return nil
}

// body decodes the value change section, starting with the already-read
// word first.
func (rd *reader) body(first []byte) error {
	// dump is the currently open dump block, if any.
	var dump DumpKind
	inDump := false

	rd.pushBack(first)
	for {
		w, ok := rd.nextWord()
		if !ok {
			break
		}
		if len(w) == 0 {
			continue
		}
		c := w[0]
		switch {
		case c == '#':
			if len(w) < 2 {
				return rd.errorf("timestamp with no digits: %q", w)
			}
			ts, err := parseUint(w[1:])
			if err != nil {
				return rd.errorf("bad timestamp %q: %v", w, err)
			}
			if err := rd.h.Timestamp(ts, w); err != nil {
				return err
			}
		case c == '$':
			name := string(w)
			if k, isDump := dumpKeywords[name]; isDump {
				if inDump {
					return rd.errorf("%v inside %v", name, dump)
				}
				inDump, dump = true, k
				if err := rd.h.DumpBegin(k); err != nil {
					return err
				}
				continue
			}
			if name == "$end" {
				if !inDump {
					return rd.errorf("$end with no open dump block")
				}
				inDump = false
				if err := rd.h.DumpEnd(dump); err != nil {
					return err
				}
				continue
			}
			// Any other `$word` opens a block that runs to `$end`.
			if err := rd.directive(name); err != nil {
				return err
			}
		case c == 'b' || c == 'B':
			if err := rd.vector(ValueBinary, w, binaryValueChars); err != nil {
				return err
			}
		case c == 'r' || c == 'R':
			if err := rd.vector(ValueReal, w, realValueChars); err != nil {
				return err
			}
		case c == 's' || c == 'S':
			// A string value may hold any printable byte, so it is
			// never split: the code always comes from the next word.
			if err := rd.emitOrWait(ValueString, w, len(w)); err != nil {
				return err
			}
		case inSet(scalarValueChars, c):
			if rd.wantPos {
				rd.pos.SetPos(rd.wordPos)
			}
			// Scalar values are normally glued to their code:
			// `0!`, `x*@`.
			if len(w) > 1 {
				if err := rd.h.ValueChange(ValueScalar, w[:1], w[1:]); err != nil {
					return err
				}
				continue
			}
			// A lone value character takes the next word as its
			// code, unless that word starts a command of its own.
			// This is what the grammar's `ValueT IdCode`
			// alternative accepts.
			rd.one[0] = c
			code, ok := rd.nextWord()
			if ok && (code[0] == '$' || code[0] == '#') {
				rd.pushBack(code)
				ok = false
			}
			if !ok {
				code = nil
			}
			if err := rd.h.ValueChange(ValueScalar, rd.one[:1], code); err != nil {
				return err
			}
		default:
			return rd.errorf("unexpected word %q in value change section", w)
		}
	}
	if inDump {
		return rd.errorf("unterminated %v block", dump)
	}
	return nil
}

// vector handles a `b`- or `r`-prefixed value, which may be glued to its
// identifier code.
func (rd *reader) vector(kind ValueKind, w []byte, set string) error {
	i := 1
	for i < len(w) && inSet(set, w[i]) {
		i++
	}
	return rd.emitOrWait(kind, w, i)
}

// emitOrWait reports a value change whose code is w[end:], or, when the
// value runs to the end of the word, reads the next word as the code.
func (rd *reader) emitOrWait(kind ValueKind, w []byte, end int) error {
	if end < len(w) {
		return rd.h.ValueChange(kind, w[:end], w[end:])
	}
	// The code is the next word, taken verbatim with no dispatch: a code
	// may spell a timestamp (`#0`), a real (`R0`) or a keyword (`$end`).
	rd.value = append(rd.value[:0], w...)
	code, ok := rd.nextWord()
	if !ok {
		return rd.errorf("value %q with no identifier code", rd.value)
	}
	return rd.h.ValueChange(kind, rd.value, code)
}

// directive consumes a `$name ... $end` block and reports it.
func (rd *reader) directive(name string) error {
	rd.text = rd.text[:0]
	for {
		w, ok := rd.nextWord()
		if !ok {
			return rd.errorf("unterminated %v block", name)
		}
		if string(w) == "$end" {
			break
		}
		if len(rd.text) > 0 {
			rd.text = append(rd.text, ' ')
		}
		rd.text = append(rd.text, w...)
	}
	return rd.h.Directive(name, rd.text)
}
