// SPDX-License-Identifier: Apache-2.0

package fst

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManyTimes builds a dump whose time table is long enough to compress,
// so a test can put a deflate stream where the raw table belongs.
func writeManyTimes(t *testing.T, path string, n int) {
	t.Helper()
	steps := []wstep{
		{openScope: "top"},
		{variable: &wsignal{name: "clk", length: 1}},
		{closeScope: true},
	}
	for i := 0; i < n; i++ {
		steps = append(steps,
			wstep{time: uint64(i), timeValid: true},
			wstep{change: &wchange{index: 0, value: map[bool]string{true: "1", false: "0"}[i%2 == 0]}})
	}
	if err := writeTestFST(path, -12, steps); err != nil {
		t.Fatalf("writeTestFST: %v", err)
	}
}

// corruptTimeTable makes a block's time table claim it is stored raw while
// it still holds the deflate stream.
//
// That is the file the bug report describes. FST has no flag saying whether
// the table is compressed: libfst writes the compressed bytes only when they
// are shorter, and the reader infers "compressed" from the stored length
// differing from the uncompressed length. A writer whose compressed table
// came out exactly as long as the raw one produces this file, and setting the
// two lengths equal in the trailer reproduces it without needing that
// coincidence. Only the trailer changes, so the block keeps its length.
func corruptTimeTable(t *testing.T, path string) {
	t.Helper()
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for pos := 0; pos < len(d); {
		seclen := int(binary.BigEndian.Uint64(d[pos+1 : pos+9]))
		end := pos + 1 + seclen
		if d[pos] == blkVCData || d[pos] == blkVCDataDynAlias || d[pos] == blkVCDataDynAlias2 {
			tr := end - timeTableTrailerLen
			uclen := binary.BigEndian.Uint64(d[tr : tr+8])
			clen := binary.BigEndian.Uint64(d[tr+8 : tr+16])
			if uclen == clen {
				t.Fatalf("the fixture's table is stored raw (%d bytes); "+
					"this corruption needs one libfst chose to compress", uclen)
			}
			// Say the uncompressed length is the stored length, which is
			// what "stored raw" looks like to the reader.
			binary.BigEndian.PutUint64(d[tr:tr+8], clen)
			if err := os.WriteFile(path, d, 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
		pos = end
	}
	t.Fatal("no value change block found in the fixture")
}

// TestParseRejectsMisflaggedTimeTable is the regression test for
// github.com/filmil/go-vcd-parser/issues/31.
//
// Before the check, libfst decoded the deflate stream as varints and reported
// ascending timestamps that looked like a real simulation, so the conversion
// succeeded and every timestamp in the database was wrong.
func TestParseRejectsMisflaggedTimeTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.fst")
	writeManyTimes(t, path, 12)
	corruptTimeTable(t, path)

	err := Parse(path, &recorder{})
	if err == nil {
		t.Fatal("Parse accepted a time table that cannot be what its trailer describes")
	}
	if !strings.Contains(err.Error(), "time table") {
		t.Errorf("Parse error = %v, want it to name the time table", err)
	}
	t.Logf("reported: %v", err)
}

// TestParseAcceptsEveryTimeChangeCount guards against the check rejecting
// files that are fine. The report notes that the writer bug showed up at one
// specific count and not at its neighbours, so the count is what to sweep.
func TestParseAcceptsEveryTimeChangeCount(t *testing.T) {
	dir := t.TempDir()
	for n := 1; n <= 64; n++ {
		path := filepath.Join(dir, "ok.fst")
		writeManyTimes(t, path, n)

		var r recorder
		if err := Parse(path, &r); err != nil {
			t.Fatalf("%d time changes: Parse: %v", n, err)
		}

		// The timestamps have to be the ones written, not merely plausible.
		var got []string
		for _, e := range r.ev {
			if strings.HasPrefix(e, "time ") {
				got = append(got, strings.Fields(e)[1])
			}
		}
		if len(got) != n {
			t.Fatalf("%d time changes: got %d timestamps %v", n, len(got), got)
		}
		for i, ts := range got {
			if ts != itoa(i) {
				t.Fatalf("%d time changes: timestamp %d is %s, want %d", n, i, ts, i)
			}
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// writeIrregularTimes builds a dump whose time deltas vary, so its table
// compresses badly and the stored length stays close to the raw one.
func writeIrregularTimes(t *testing.T, path string, n int) []uint64 {
	t.Helper()
	steps := []wstep{
		{openScope: "top"},
		{variable: &wsignal{name: "clk", length: 1}},
		{closeScope: true},
	}
	var times []uint64
	tm := uint64(0)
	for i := 0; i < n; i++ {
		// Deltas spread over several varint widths.
		tm += uint64(1+i*7919%65536) * uint64(1+i%3)
		times = append(times, tm)
		steps = append(steps,
			wstep{time: tm, timeValid: true},
			wstep{change: &wchange{index: 0, value: map[bool]string{true: "1", false: "0"}[i%2 == 0]}})
	}
	if err := writeTestFST(path, -12, steps); err != nil {
		t.Fatalf("writeTestFST: %v", err)
	}
	return times
}

// corruptRawTimeTable overwrites a table that is stored raw with a deflate
// stream of the same length, leaving the trailer alone.
//
// This is the other shape of the reported file: the lengths already agree, so
// the reader reads the bytes as varints, and they are not varints. A table of
// wide entries compresses badly enough that libfst stores it raw, which is
// what makes this corruption possible without resizing the block.
func corruptRawTimeTable(t *testing.T, path string) {
	t.Helper()
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for pos := 0; pos < len(d); {
		seclen := int(binary.BigEndian.Uint64(d[pos+1 : pos+9]))
		end := pos + 1 + seclen
		if d[pos] == blkVCData || d[pos] == blkVCDataDynAlias || d[pos] == blkVCDataDynAlias2 {
			tr := end - timeTableTrailerLen
			uclen := int(binary.BigEndian.Uint64(d[tr : tr+8]))
			clen := int(binary.BigEndian.Uint64(d[tr+8 : tr+16]))
			if uclen != clen {
				t.Fatalf("the fixture's table is stored compressed (%d vs %d); "+
					"this corruption needs one libfst stored raw", uclen, clen)
			}
			var buf bytes.Buffer
			zw := zlib.NewWriter(&buf)
			zw.Write(d[tr-clen : tr])
			zw.Close()
			z := buf.Bytes()
			for len(z) < clen {
				z = append(z, 0)
			}
			copy(d[tr-clen:tr], z[:clen])
			if err := os.WriteFile(path, d, 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
		pos = end
	}
	t.Fatal("no value change block found in the fixture")
}

// TestParseRejectsTableWithWrongByteCount covers the other way a misflagged
// table shows up: enough declared bytes for the entries, but reading them
// does not land on the end of the table. The check that catches this one is
// the byte count, not the entry count.
func TestParseRejectsTableWithWrongByteCount(t *testing.T) {
	dir := t.TempDir()
	checked := 0
	for n := 8; n <= 40; n++ {
		path := filepath.Join(dir, "bad.fst")
		writeIrregularTimes(t, path, n)

		d, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		uclen, clen, nitems, ok := firstTimeTableTrailer(d)
		if !ok || uclen != clen || nitems > uclen {
			continue // Not the shape this test is about.
		}
		corruptRawTimeTable(t, path)

		err = Parse(path, &recorder{})
		if err == nil {
			t.Fatalf("%d times: Parse accepted a raw table holding a deflate stream "+
				"(%d entries, %d bytes)", n, nitems, uclen)
		}
		if !strings.Contains(err.Error(), "time table") {
			t.Fatalf("%d times: error = %v, want it to name the time table", n, err)
		}
		if checked == 0 {
			t.Logf("%d times: %v", n, err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no time change count produced a raw time table, so this check never ran")
	}
	t.Logf("checked %d counts", checked)
}

// firstTimeTableTrailer returns the trailer of the first value change block.
func firstTimeTableTrailer(d []byte) (uclen, clen, nitems uint64, ok bool) {
	for pos := 0; pos+blockHeaderLen <= len(d); {
		seclen := int(binary.BigEndian.Uint64(d[pos+1 : pos+9]))
		end := pos + 1 + seclen
		if seclen <= 0 || end > len(d) {
			return 0, 0, 0, false
		}
		if d[pos] == blkVCData || d[pos] == blkVCDataDynAlias || d[pos] == blkVCDataDynAlias2 {
			tr := end - timeTableTrailerLen
			return binary.BigEndian.Uint64(d[tr : tr+8]),
				binary.BigEndian.Uint64(d[tr+8 : tr+16]),
				binary.BigEndian.Uint64(d[tr+16 : tr+24]), true
		}
		pos = end
	}
	return 0, 0, 0, false
}

// TestParseAcceptsIrregularTimes checks the sweep above against uncorrupted
// files, so the byte-count check is not rejecting wide time tables.
func TestParseAcceptsIrregularTimes(t *testing.T) {
	dir := t.TempDir()
	for n := 1; n <= 40; n++ {
		path := filepath.Join(dir, "ok.fst")
		want := writeIrregularTimes(t, path, n)

		var r recorder
		if err := Parse(path, &r); err != nil {
			t.Fatalf("%d times: Parse: %v", n, err)
		}
		var got []string
		for _, e := range r.ev {
			if strings.HasPrefix(e, "time ") {
				got = append(got, strings.Fields(e)[1])
			}
		}
		if len(got) != len(want) {
			t.Fatalf("%d times: got %d timestamps, want %d", n, len(got), len(want))
		}
		for i := range want {
			if got[i] != itoa(int(want[i])) {
				t.Fatalf("%d times: timestamp %d is %s, want %d", n, i, got[i], want[i])
			}
		}
	}
}
