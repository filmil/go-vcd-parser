// SPDX-License-Identifier: Apache-2.0

package fst

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// FST block types, from fstapi.h. Only the ones this file reasons about are
// named; every other type is stepped over.
const (
	blkVCData          = 1
	blkVCDataDynAlias  = 5
	blkVCDataDynAlias2 = 8
)

// blockHeaderLen is the type byte plus the eight bytes of section length.
// A block at offset p runs to p+1+seclen.
const blockHeaderLen = 9

// timeTableTrailerLen is the three uint64 fields that close a value change
// block: the uncompressed length, the stored length, and the number of
// entries.
const timeTableTrailerLen = 24

// validateTimeTables reports an error when a value change block's time table
// cannot be the table its trailer describes.
//
// It exists because of how FST says whether the table is compressed. There is
// no flag: libfst writes the compressed bytes only when they are shorter than
// the raw ones, and the reader infers "compressed" from the stored length
// being different from the uncompressed length. A writer that emits a
// compressed table whose length happens to equal the uncompressed length
// therefore produces a file that libfst reads as raw varints. The deltas it
// decodes out of the deflate stream are plausible ascending numbers, so the
// dump converts without complaint and every timestamp in it is wrong. The
// first delta is 0x78, the deflate header byte, which is why such a dump
// starts 0, 120.
//
// The invariant that catches it: decoding the declared number of entries from
// a raw table consumes exactly the declared number of bytes. A deflate stream
// read as varints does not.
//
// The check is deliberately one-sided. A definite violation is an error;
// anything this cannot make sense of, such as an unfamiliar block type or a
// block list that does not add up, is left to libfst rather than being called
// corrupt here.
func validateTimeTables(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	for pos := int64(0); pos < size; {
		var head [blockHeaderLen]byte
		if _, err := f.ReadAt(head[:], pos); err != nil {
			return nil // Not a shape this understands; libfst decides.
		}
		seclen := int64(binary.BigEndian.Uint64(head[1:]))
		end := pos + 1 + seclen
		if seclen <= 0 || end > size {
			return nil
		}

		switch head[0] {
		case blkVCData, blkVCDataDynAlias, blkVCDataDynAlias2:
			if err := validateBlockTimeTable(f, pos, end); err != nil {
				return err
			}
		}
		pos = end
	}
	return nil
}

// validateBlockTimeTable checks the time table of one value change block,
// which occupies [pos, end).
func validateBlockTimeTable(f *os.File, pos, end int64) error {
	if end-pos < timeTableTrailerLen+blockHeaderLen {
		return nil
	}
	var trailer [timeTableTrailerLen]byte
	if _, err := f.ReadAt(trailer[:], end-timeTableTrailerLen); err != nil {
		return nil
	}
	uclen := binary.BigEndian.Uint64(trailer[0:8])
	clen := binary.BigEndian.Uint64(trailer[8:16])
	nitems := binary.BigEndian.Uint64(trailer[16:24])

	// The stored table sits immediately before the trailer.
	start := end - timeTableTrailerLen - int64(clen)
	if clen > uint64(end-pos) || start < pos+blockHeaderLen {
		return fmt.Errorf(
			"the time table of the value change block at offset %d does not fit in it: "+
				"it claims %d stored bytes in a %d byte block", pos, clen, end-pos)
	}
	// An entry is at least one byte, so a table cannot hold more entries
	// than it has bytes.
	if nitems > uclen {
		return fmt.Errorf(
			"the time table of the value change block at offset %d claims %d entries "+
				"in %d uncompressed bytes", pos, nitems, uclen)
	}
	if nitems == 0 {
		return nil
	}

	stored := make([]byte, clen)
	if _, err := f.ReadAt(stored, start); err != nil {
		return nil
	}

	if clen != uclen {
		// Different lengths mean deflate. Checking that it inflates to the
		// declared size catches a truncated table.
		return checkDeflated(pos, stored, uclen)
	}

	// Equal lengths mean the table is stored raw, so it must be exactly
	// nitems varints long.
	used, err := varintBytes(stored, nitems)
	if err != nil {
		return fmt.Errorf(
			"the time table of the value change block at offset %d is not readable as "+
				"%d entries: %w. A time table stored uncompressed but holding compressed "+
				"bytes reads this way, and FST has no flag that would tell them apart",
			pos, nitems, err)
	}
	if used != int(uclen) {
		return fmt.Errorf(
			"the time table of the value change block at offset %d declares %d entries "+
				"in %d bytes, but reading that many entries uses %d bytes. A time table "+
				"stored uncompressed but holding compressed bytes reads this way, and FST "+
				"has no flag that would tell them apart",
			pos, nitems, uclen, used)
	}
	return nil
}

// checkDeflated reports an error when stored does not inflate to uclen bytes.
func checkDeflated(pos int64, stored []byte, uclen uint64) error {
	zr, err := zlib.NewReader(bytes.NewReader(stored))
	if err != nil {
		return fmt.Errorf(
			"the time table of the value change block at offset %d is stored compressed "+
				"but is not a zlib stream: %w", pos, err)
	}
	defer zr.Close()
	n, err := io.Copy(io.Discard, zr)
	if err != nil {
		return fmt.Errorf(
			"the time table of the value change block at offset %d does not decompress: %w",
			pos, err)
	}
	if uint64(n) != uclen {
		return fmt.Errorf(
			"the time table of the value change block at offset %d decompresses to %d "+
				"bytes, but declares %d", pos, n, uclen)
	}
	return nil
}

// varintBytes returns how many bytes n consecutive varints occupy, and fails
// rather than reading past the end of b.
func varintBytes(b []byte, n uint64) (int, error) {
	i := 0
	for k := uint64(0); k < n; k++ {
		shift := 0
		for {
			if i >= len(b) {
				return 0, fmt.Errorf("entry %d runs past the end of the table", k)
			}
			c := b[i]
			i++
			if c&0x80 == 0 {
				break
			}
			shift += 7
			if shift >= 64 {
				return 0, fmt.Errorf("entry %d is longer than 64 bits", k)
			}
		}
	}
	return i, nil
}
