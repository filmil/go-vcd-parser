package files

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/filmil/go-vcd-parser/cvt"
	"github.com/filmil/go-vcd-parser/db"
	"github.com/filmil/go-vcd-parser/vcd"
)

const benchFile = "samples/tb.vcd"

// BenchmarkParse measures decoding alone, with a handler that does no
// work, so that the parser can be told apart from whatever consumes it.
func BenchmarkParse(b *testing.B) {
	data := readSample(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := vcd.Parse(benchFile, newReader(data), &vcd.NopHandler{}); err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}

// newReader hands out data without copying it, so the benchmark measures
// the parser rather than the allocator.
func newReader(data []byte) *sliceReader { return &sliceReader{data: data} }

type sliceReader struct {
	data []byte
	off  int
}

func (s *sliceReader) Read(p []byte) (int, error) {
	if s.off >= len(s.data) {
		return 0, io.EOF
	}
	n := copy(p, s.data[s.off:])
	s.off += n
	return n, nil
}

// BenchmarkParseFile measures building the whole parse tree through the
// streaming reader. The gap to BenchmarkParse is what the tree costs.
func BenchmarkParseFile(b *testing.B) {
	data := readSample(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := vcd.ParseFile(benchFile, newReader(data)); err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}

// BenchmarkParseGrammar measures the grammar-driven parser, which holds
// the input text, the whole token stream and the tree at once. It is here
// as the baseline the streaming reader replaced: compare B/op.
func BenchmarkParseGrammar(b *testing.B) {
	data := readSample(b)
	parser := vcd.NewParser[vcd.File]()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := parser.Parse(benchFile, newReader(data)); err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}

// BenchmarkConvertStream measures a whole conversion, VCD file to signals
// database, which is what `vcdcvt --format=sqlite` does. This is the
// number that matters: parsing is a few per cent of it, and the database
// write is the rest.
func BenchmarkConvertStream(b *testing.B) {
	data := readSample(b)
	ctx := context.Background()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := filepath.Join(b.TempDir(), fmt.Sprintf("bench.%d.db", i))
		dbf, err := db.OpenBulk(ctx, name)
		if err != nil {
			b.Fatalf("could not open: %v", err)
		}
		if err := cvt.ConvertStream(ctx, benchFile, newReader(data), dbf); err != nil {
			b.Fatalf("could not convert: %v", err)
		}
		if err := db.FinishBulk(ctx, dbf); err != nil {
			b.Fatalf("could not finish: %v", err)
		}
		dbf.Close()
	}
	b.StopTimer()
	if mb, ok := residentMB(); ok {
		b.ReportMetric(mb, "rss_MB")
	}
}

func readSample(b *testing.B) []byte {
	b.Helper()
	data, err := os.ReadFile(benchFile)
	if err != nil {
		b.Fatalf("could not read %v: %v", benchFile, err)
	}
	return data
}

// residentMB reads the current resident set size, which covers the memory
// SQLite allocates from C as well as Go's own heap. It is process-wide, so
// it only means anything when one benchmark runs on its own.
func residentMB() (float64, bool) {
	buf, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false // not Linux
	}
	var total, resident int64
	if _, err := fmt.Sscan(string(buf), &total, &resident); err != nil {
		return 0, false
	}
	return float64(resident*int64(os.Getpagesize())) / (1 << 20), true
}
