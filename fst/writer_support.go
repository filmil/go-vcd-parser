// SPDX-License-Identifier: Apache-2.0

package fst

/*
#include <stdlib.h>
#include "fstapi.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// This file exists because rules_go does not support cgo in a _test.go file,
// and the round-trip test needs a dump in the real format. Writing one with
// libfst's own writer is the only way to be sure the reader is tested
// against what a real writer produces, rather than against a hand-made
// approximation that could share a wrong assumption with the reader.
//
// Nothing here is exported, so it does not widen the package's API.

// wsignal is one variable in a dump built by writeTestFST.
type wsignal struct {
	name   string
	length int
	// real declares the variable as FST_VT_VCD_REAL, whose value changes
	// are written as a native double rather than as one character per bit.
	real bool
}

// wchange is one value change at a given time, naming the variable by its
// position in the signal list.
type wchange struct {
	time  uint64
	index int
	value string
	// realValue is written instead of value when the variable is a real.
	realValue float64
}

// writeTestFST builds an FST dump at path with libfst's writer.
//
// scopes is walked in order: a non-empty entry opens a scope, and an empty
// entry closes one. Each signal names the scope depth it belongs to by
// appearing between the entries that open and close it.
func writeTestFST(path string, timescaleExp int, steps []wstep) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	w := C.fstWriterCreate(cpath, 1)
	if w == nil {
		return fmt.Errorf("fstWriterCreate: %v: failed", path)
	}
	C.fstWriterSetTimescale(w, C.int(timescaleExp))

	var handles []C.fstHandle
	var isReal []bool
	for _, s := range steps {
		switch {
		case s.openScope != "":
			n := C.CString(s.openScope)
			C.fstWriterSetScope(w, C.FST_ST_VCD_MODULE, n, nil)
			C.free(unsafe.Pointer(n))
		case s.closeScope:
			C.fstWriterSetUpscope(w)
		case s.variable != nil:
			n := C.CString(s.variable.name)
			vt := C.enum_fstVarType(C.FST_VT_VCD_WIRE)
			if s.variable.real {
				vt = C.FST_VT_VCD_REAL
			}
			h := C.fstWriterCreateVar(w, vt, C.FST_VD_IMPLICIT,
				C.uint32_t(s.variable.length), n, 0)
			C.free(unsafe.Pointer(n))
			handles = append(handles, h)
			isReal = append(isReal, s.variable.real)
		case s.change != nil:
			c := s.change
			if c.index < 0 || c.index >= len(handles) {
				C.fstWriterClose(w)
				return fmt.Errorf("writeTestFST: change names variable %d, of %d declared", c.index, len(handles))
			}
			if isReal[c.index] {
				// A real is written as the raw double; libfst turns it
				// into text on the way back out.
				d := C.double(c.realValue)
				C.fstWriterEmitValueChange(w, handles[c.index], unsafe.Pointer(&d))
				break
			}
			v := C.CString(c.value)
			C.fstWriterEmitValueChange(w, handles[c.index], unsafe.Pointer(v))
			C.free(unsafe.Pointer(v))
		case s.timeValid:
			C.fstWriterEmitTimeChange(w, C.uint64_t(s.time))
		}
	}

	C.fstWriterClose(w)
	return nil
}

// wstep is one instruction for writeTestFST. Exactly one field is set.
type wstep struct {
	openScope  string
	closeScope bool
	variable   *wsignal
	change     *wchange
	time       uint64
	timeValid  bool
}
