// Package fst reads GTKWave FST (Fast Signal Trace) dumps.
//
// FST is the compressed binary waveform format written by GTKWave, Verilator,
// Icarus Verilog and nvc. This package reports a dump through the same
// [vcd.Handler] interface the VCD parser uses, so every sink written for VCD
// consumes an FST dump unchanged.
//
// The reading is done by libfst, the MIT-licensed C library split out of
// GTKWave, vendored under //third_party/libfst. That directory's
// LIBFST_VERSION names the revision and its LICENSE holds the license.
//
// Because the C lives outside this package, cgo does not compile it: the
// Bazel cc_library does. So this package builds under Bazel and not under a
// plain `go build`.
//
// FST is a block-based format whose value change blocks are indexed from a
// table at the end of the file, so unlike [vcd.Parse] this reader needs a
// named file it can seek in rather than an io.Reader.
//
// Timestamps arrive in increasing order. The value changes within one
// timestamp arrive in the order the block holds them, which is not the order
// the writer emitted them in. They all happen at the same instant, so no
// information is lost, but a caller that compares two dumps has to treat one
// timestamp's changes as a set.
package fst

/*
#include <stdlib.h>
#include "fstapi.h"
#include "bridge.h"
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"

	"github.com/filmil/go-vcd-parser/vcd"
)

// reader holds the state one Parse call needs while libfst drives it.
type reader struct {
	h vcd.Handler

	// sig is indexed by fstHandle, which libfst numbers from 1. Index 0 is
	// unused, so a missing handle reads as the zero value.
	sig []signal

	// tsValid guards tsCur: time zero is a legitimate timestamp, so a
	// separate flag says whether any timestamp has been reported yet.
	tsCur   uint64
	tsValid bool

	// vbuf and tbuf are reused across value changes, so a conversion does
	// not allocate per row. The Handler contract makes the slices valid
	// only for the duration of the call, which is what lets them be
	// reused at all.
	vbuf []byte
	tbuf []byte

	// err is the first error a handler returned. libfst's iterator cannot
	// be stopped from a callback, so the remaining callbacks return early
	// and Parse reports this once the iteration ends.
	err error
}

// signal is what a value change needs to know about its facility: how to
// spell the value, and which identifier code to report it under.
type signal struct {
	code   string
	length uint32
	real   bool
	str    bool
}

// Parse reads the FST dump at path and reports it to h.
//
// The events are the ones a VCD file of the same content would produce: the
// $timescale and the $scope/$var/$upscope tree as Declaration calls, then
// $enddefinitions, then Timestamp and ValueChange in time order.
//
// The []byte arguments handed to h alias a buffer owned by this call and are
// valid only for its duration, matching [vcd.Parse]. Wrap h in
// [vcd.CopyHandler] to retain them.
func Parse(path string, h vcd.Handler) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	// Check the time tables before libfst is given the file. libfst reads a
	// malformed one as plausible ascending timestamps rather than failing,
	// and a dump converted from it is wrong everywhere without looking it.
	if err := validateTimeTables(path); err != nil {
		return fmt.Errorf("fst.Parse: %v: %w", path, err)
	}

	ctx := C.fstReaderOpen(cpath)
	if ctx == nil {
		return fmt.Errorf("fst.Parse: %v: not an FST file, or it cannot be read", path)
	}
	defer C.fstReaderClose(ctx)

	r := &reader{h: h}

	if err := r.declareTimescale(ctx); err != nil {
		return fmt.Errorf("fst.Parse: %v: %w", path, err)
	}
	if err := r.declareHierarchy(ctx); err != nil {
		return fmt.Errorf("fst.Parse: %v: %w", path, err)
	}
	if err := r.declareEnd(); err != nil {
		return fmt.Errorf("fst.Parse: %v: %w", path, err)
	}

	// Every facility is wanted; without this libfst reports nothing.
	C.fstReaderSetFacProcessMaskAll(ctx)

	hnd := cgo.NewHandle(r)
	defer hnd.Delete()
	if C.goFstIterBlocks2(ctx, unsafe.Pointer(hnd)) == 0 {
		// A zero return means libfst could not walk the blocks. A handler
		// error is the more useful thing to report when both happened.
		if r.err == nil {
			return fmt.Errorf("fst.Parse: %v: could not read the value change blocks", path)
		}
	}
	if r.err != nil {
		return fmt.Errorf("fst.Parse: %v: %w", path, r.err)
	}
	return nil
}
