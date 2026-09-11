// SPDX-License-Identifier: Apache-2.0

package fst

/*
#include <stdint.h>
#include <string.h>
#include "fstapi.h"
*/
import "C"

import (
	"runtime/cgo"
	"strconv"
	"unsafe"

	"github.com/filmil/go-vcd-parser/vcd"
)

// goFstValueChange receives one fixed-length value change from libfst.
//
//export goFstValueChange
func goFstValueChange(ud unsafe.Pointer, tim C.uint64_t, facidx C.fstHandle, value *C.uchar) {
	r := cgo.Handle(ud).Value().(*reader)
	if r.err != nil {
		return
	}
	sg := r.signal(uint32(facidx))
	// A bit vector arrives as one character per bit, so the declared length
	// is the length of the value and no strlen is needed on the hot path.
	//
	// A real does not: libfst formats it with "%.16g" into a buffer, so the
	// text is as long as the number needs and the declared length, which is
	// 8 for every real, would both truncate a long one and over-read a
	// short one.
	n := int(sg.length)
	if n == 0 || sg.real {
		n = int(C.strlen((*C.char)(unsafe.Pointer(value))))
	}
	r.change(uint64(tim), sg, unsafe.Slice((*byte)(unsafe.Pointer(value)), n))
}

// goFstValueChangeVarlen receives one variable-length value change, which is
// how FST reports a generic string.
//
//export goFstValueChangeVarlen
func goFstValueChangeVarlen(ud unsafe.Pointer, tim C.uint64_t, facidx C.fstHandle, value *C.uchar, length C.uint32_t) {
	r := cgo.Handle(ud).Value().(*reader)
	if r.err != nil {
		return
	}
	sg := r.signal(uint32(facidx))
	sg.str = true
	r.change(uint64(tim), sg, unsafe.Slice((*byte)(unsafe.Pointer(value)), int(length)))
}

// signal looks up a facility, tolerating a handle the header did not declare.
func (r *reader) signal(h uint32) signal {
	if int(h) < len(r.sig) {
		return r.sig[h]
	}
	return signal{code: idCode(h)}
}

// change reports one value change, first reporting the timestamp when the
// simulation time has moved since the last one.
//
// buf holds the value as libfst spells it, without the type prefix a VCD
// file would carry. vcd.ValueKind.Payload strips that prefix for every kind
// except a scalar, so a prefix byte is put back here for the kinds that have
// one. The prefix and the value go to the handler as one slice, which is
// what the VCD parser hands over too.
func (r *reader) change(tim uint64, sg signal, buf []byte) {
	if !r.tsValid || tim != r.tsCur {
		r.tsCur, r.tsValid = tim, true
		if err := r.h.Timestamp(tim, r.timeText(tim)); err != nil {
			r.err = err
			return
		}
	}

	kind := vcd.ValueScalar
	switch {
	case sg.str:
		kind = vcd.ValueString
	case sg.real:
		kind = vcd.ValueReal
	case len(buf) > 1:
		kind = vcd.ValueBinary
	}

	var value []byte
	if kind == vcd.ValueScalar {
		value = buf
	} else {
		// One reused buffer: the handler contract says the slice is only
		// valid for the call, exactly as the VCD parser promises.
		r.vbuf = append(r.vbuf[:0], prefix(kind))
		r.vbuf = append(r.vbuf, buf...)
		value = r.vbuf
	}

	if err := r.h.ValueChange(kind, value, []byte(sg.code)); err != nil {
		r.err = err
	}
}

// prefix is the character a VCD file puts in front of a non-scalar value.
func prefix(k vcd.ValueKind) byte {
	switch k {
	case vcd.ValueBinary:
		return 'b'
	case vcd.ValueReal:
		return 'r'
	case vcd.ValueString:
		return 's'
	}
	return 0
}

// timeText spells a timestamp the way the VCD token for it reads, which is
// what a Handler is given alongside the parsed value. The buffer is reused
// across calls, on the same terms as the value buffer.
func (r *reader) timeText(tim uint64) []byte {
	r.tbuf = strconv.AppendUint(r.tbuf[:0], tim, 10)
	return r.tbuf
}
