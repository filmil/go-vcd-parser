package vcd

import (
	"fmt"
	"io"

	"github.com/alecthomas/participle/v2/lexer"
)

// MaxWordSize is the largest single whitespace-delimited word the scanner
// will return. VCD words are short -- an identifier code, a value, a
// keyword -- so this only ever trips on a malformed or binary file, where
// erroring out beats growing the buffer without bound.
const MaxWordSize = 1 << 20

// initialBufSize is the starting scanner buffer. Words are far smaller than
// this; the size is chosen to keep the number of Read calls down.
const initialBufSize = 64 << 10

// isSpace reports whether c separates two VCD words.
//
// This is the complement of the lexer's AnyWordPattern, which excludes
// `\r\n\t\f\v` and a space. Note that RE2's `\s` does not include `\v`,
// so the regex lexer rejects a file containing one; the scanner treats it
// as the separator it is.
func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// wordScanner splits an io.Reader into whitespace-delimited words.
//
// The []byte returned by Next aliases the scanner's own buffer and is only
// valid until the following call to Next.
type wordScanner struct {
	r   io.Reader
	buf []byte

	// start is where the unconsumed data in buf begins, end where it ends.
	start, end int

	err error // sticky read error, returned once the buffer drains
	eof bool

	filename string

	// offset, line and column track the position of buf[start]. Line and
	// column are 1-based and column counts runes, matching participle's
	// lexer.Position so that positions can be compared against it.
	offset, line, column int

	// pos is the position of the word most recently returned by Next.
	pos lexer.Position

	// While recording, every consumed byte is copied to rec. The header
	// is parsed from those original bytes rather than from re-joined
	// words, because the grammar's `$end` token is `\$end(\s+|$)` --
	// it captures its own trailing whitespace, so respacing the input
	// would change the strings the grammar captures.
	recording bool
	rec       []byte
}

func newWordScanner(filename string, r io.Reader) *wordScanner {
	return &wordScanner{
		r:        r,
		buf:      make([]byte, initialBufSize),
		filename: filename,
		line:     1,
		column:   1,
	}
}

// advance moves the tracked position over p, which must be about to be
// consumed. It mirrors lexer.Position.Advance: bytes for the offset, "\n"
// for the line, and runes since the last "\n" for the column.
func (s *wordScanner) advance(p []byte) {
	if s.recording {
		s.rec = append(s.rec, p...)
	}
	for _, c := range p {
		s.offset++
		switch {
		case c == '\n':
			s.line++
			s.column = 1
		case c&0xC0 != 0x80: // not a UTF-8 continuation byte
			s.column++
		}
	}
}

// fill reads more data into buf, first sliding any unconsumed bytes to the
// front and growing the buffer if it is already full.
func (s *wordScanner) fill() error {
	if s.eof {
		return io.EOF
	}
	if s.start > 0 {
		copy(s.buf, s.buf[s.start:s.end])
		s.end -= s.start
		s.start = 0
	}
	if s.end == len(s.buf) {
		if len(s.buf) >= MaxWordSize {
			return fmt.Errorf("%v: word longer than %d bytes at %v",
				s.filename, MaxWordSize, s.Pos())
		}
		size := len(s.buf) * 2
		if size > MaxWordSize {
			size = MaxWordSize
		}
		grown := make([]byte, size)
		copy(grown, s.buf[:s.end])
		s.buf = grown
	}
	n, err := s.r.Read(s.buf[s.end:])
	s.end += n
	if err != nil {
		s.eof = true
		if err != io.EOF {
			s.err = err
		}
		if n == 0 {
			return io.EOF
		}
	}
	return nil
}

// skipSpace consumes leading whitespace, reporting whether any data remains.
func (s *wordScanner) skipSpace() bool {
	for {
		for s.start < s.end && isSpace(s.buf[s.start]) {
			s.advance(s.buf[s.start : s.start+1])
			s.start++
		}
		if s.start < s.end {
			return true
		}
		if err := s.fill(); err != nil {
			return false
		}
	}
}

// Next returns the next word, or false at end of input. The returned slice
// aliases the scanner buffer and is invalidated by the following call.
func (s *wordScanner) Next() ([]byte, bool) {
	if !s.skipSpace() {
		return nil, false
	}
	s.pos = s.Pos()
	// Extend the word until a separator shows up in the buffer. n counts the
	// bytes already known to belong to the word, so a refill does not rescan.
	n := 0
	for {
		for s.start+n < s.end && !isSpace(s.buf[s.start+n]) {
			n++
		}
		if s.start+n < s.end {
			break // hit a separator
		}
		if err := s.fill(); err != nil {
			break // hit end of input
		}
	}
	word := s.buf[s.start : s.start+n]
	s.advance(word)
	s.start += n
	return word, true
}

// dropLast removes the n bytes most recently recorded, used to hand back a
// word that turned out to belong to the next section.
func (s *wordScanner) dropLast(n int) {
	s.rec = s.rec[:len(s.rec)-n]
}

// Pos is the position of the next byte to be read.
func (s *wordScanner) Pos() lexer.Position {
	return lexer.Position{
		Filename: s.filename,
		Offset:   s.offset,
		Line:     s.line,
		Column:   s.column,
	}
}

// WordPos is the position of the word most recently returned by Next.
func (s *wordScanner) WordPos() lexer.Position { return s.pos }

// Err reports a read error from the underlying reader, if any. End of input
// is not an error.
func (s *wordScanner) Err() error { return s.err }
