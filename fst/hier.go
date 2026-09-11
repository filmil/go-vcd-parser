// SPDX-License-Identifier: Apache-2.0

package fst

/*
#include <stdlib.h>
#include "fstapi.h"
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/filmil/go-vcd-parser/vcd"
)

// scopeOf and varOf read the arm of struct fstHier's union that htyp
// selects. cgo represents a C union as raw bytes, so the arm has to be
// named by a cast rather than by a field.
func scopeOf(e *C.struct_fstHier) *C.struct_fstHierScope {
	return (*C.struct_fstHierScope)(unsafe.Pointer(&e.u))
}

func varOf(e *C.struct_fstHier) *C.struct_fstHierVar {
	return (*C.struct_fstHierVar)(unsafe.Pointer(&e.u))
}

// declareTimescale reports the dump's time unit as a $timescale.
//
// FST stores one signed exponent: the number of seconds a tick counts is
// 10**exp. VCD spells the same thing as a number and a unit, and only 1, 10
// and 100 are legal numbers, so the unit is the largest power of 1000 at or
// below the exponent and the number is what remains.
func (r *reader) declareTimescale(ctx *C.fstReaderContext) error {
	exp := int(C.fstReaderGetTimescale(ctx))
	unit, unitExp := timeUnit(exp)
	if unit == nil {
		return fmt.Errorf("unsupported timescale exponent %d", exp)
	}
	n := int64(1)
	for i := 0; i < exp-unitExp; i++ {
		n *= 10
	}
	return r.h.Declaration(&vcd.DeclarationCommandT{
		Timescale: &vcd.TimescaleT{Kw: true, Number: n, Unit: unit, Kw2: true},
	})
}

// timeUnit picks the VCD unit for a power-of-ten exponent of seconds, and
// returns the exponent that unit itself stands for. An exponent below the
// femtosecond, or above the second, has no VCD spelling.
func timeUnit(exp int) (*vcd.TimeUnit, int) {
	switch {
	case exp >= 0:
		return &vcd.TimeUnit{Second: true}, 0
	case exp >= -3:
		return &vcd.TimeUnit{MilliSecond: true}, -3
	case exp >= -6:
		return &vcd.TimeUnit{MicroSecond: true}, -6
	case exp >= -9:
		return &vcd.TimeUnit{NanoSecond: true}, -9
	case exp >= -12:
		return &vcd.TimeUnit{PicoSecond: true}, -12
	case exp >= -15:
		return &vcd.TimeUnit{FemtoSecond: true}, -15
	}
	return nil, 0
}

// declareHierarchy walks the scope and variable tree, reporting each entry
// as the $scope, $upscope or $var a VCD file would carry, and records what
// each facility needs for its value changes.
func (r *reader) declareHierarchy(ctx *C.fstReaderContext) error {
	// Handles count from 1, so the table is one longer than the count.
	r.sig = make([]signal, C.fstReaderGetVarCount(ctx)+1)

	for {
		e := C.fstReaderIterateHier(ctx)
		if e == nil {
			return nil
		}
		switch e.htyp {
		case C.FST_HT_SCOPE:
			s := scopeOf(e)
			d := &vcd.DeclarationCommandT{Scope: &vcd.ScopeT{
				Scope:     true,
				ScopeKind: scopeKind(byte(s.typ)),
				Id:        C.GoStringN(s.name, C.int(s.name_length)),
				KwEnd:     true,
			}}
			if err := r.h.Declaration(d); err != nil {
				return err
			}
		case C.FST_HT_UPSCOPE:
			up := true
			if err := r.h.Declaration(&vcd.DeclarationCommandT{Upscope: &up}); err != nil {
				return err
			}
		case C.FST_HT_VAR:
			v := varOf(e)
			if err := r.declareVar(v); err != nil {
				return err
			}
		}
		// FST_HT_ATTRBEGIN and FST_HT_ATTREND carry writer metadata that
		// has no bearing on the signals or their values.
	}
}

// declareVar reports one $var and records its facility.
func (r *reader) declareVar(v *C.struct_fstHierVar) error {
	h := uint32(v.handle)
	if int(h) >= len(r.sig) {
		// A handle past the declared variable count means the header and
		// the geometry disagree. Grow rather than drop the signal.
		grown := make([]signal, h+1)
		copy(grown, r.sig)
		r.sig = grown
	}

	name, indices := splitIndices(C.GoStringN(v.name, C.int(v.name_length)))
	typ := varType(byte(v.typ))
	sg := signal{
		code:   idCode(h),
		length: uint32(v.length),
		real:   typ == "real",
		str:    typ == "string",
	}
	r.sig[h] = sg

	return r.h.Declaration(&vcd.DeclarationCommandT{
		Var: &vcd.VarT{
			Kw:      true,
			VarType: typ,
			Size:    int(v.length),
			Code:    sg.code,
			Id:      vcd.IdT{Name: name, Indices: indices},
			KwEnd:   true,
		},
	})
}

// declareEnd closes the header.
func (r *reader) declareEnd() error {
	end := true
	return r.h.Declaration(&vcd.DeclarationCommandT{EndDefinitions: &end})
}

// splitIndices peels a trailing bit range off an FST variable name. libfst
// reports a vector as `data [7:0]`, where VCD writes the range as part of
// the identifier. Returning it as indices keeps the name the sink stores
// identical to the one the same dump in VCD form would give.
func splitIndices(s string) (string, []*vcd.IdxT) {
	i := strings.LastIndex(s, " [")
	if i < 0 || !strings.HasSuffix(s, "]") {
		return s, nil
	}
	inner := s[i+2 : len(s)-1]
	name := s[:i]

	if msb, lsb, ok := strings.Cut(inner, ":"); ok {
		m, err1 := strconv.Atoi(msb)
		l, err2 := strconv.Atoi(lsb)
		if err1 != nil || err2 != nil {
			return s, nil
		}
		return name, []*vcd.IdxT{{MsbIndex: &m, LsbIndex: &l}}
	}
	n, err := strconv.Atoi(inner)
	if err != nil {
		return s, nil
	}
	return name, []*vcd.IdxT{{Index: &n}}
}

// idCodeAlphabet is the printable ASCII range VCD uses for identifier codes.
const idCodeAlphabet = 94

// idCode spells an FST handle as a VCD identifier code. FST identifies a
// facility by a number and VCD by a short string of printable characters, so
// the number is written in base 94 over that alphabet. The mapping is
// one-to-one, which is what aliased facilities need: FST gives two names the
// same handle, and they must reach the sink under the same code.
func idCode(h uint32) string {
	var b []byte
	for {
		b = append(b, byte('!'+h%idCodeAlphabet))
		h /= idCodeAlphabet
		if h == 0 {
			return string(b)
		}
	}
}

// scopeKind maps an FST scope type to the VCD keyword for it. VCD has five
// scope keywords; FST has more, and the ones with no VCD spelling become
// `module`, which is what a VCD writer emits for them.
func scopeKind(t byte) vcd.ScopeKindT {
	switch t {
	case C.FST_ST_VCD_TASK:
		return vcd.ScopeKindT{Task: true}
	case C.FST_ST_VCD_FUNCTION:
		return vcd.ScopeKindT{Function: true}
	case C.FST_ST_VCD_BEGIN:
		return vcd.ScopeKindT{Begin: true}
	case C.FST_ST_VCD_FORK:
		return vcd.ScopeKindT{Fork: true}
	}
	return vcd.ScopeKindT{Module: true}
}

// fstVarTypes is the VCD keyword for each FST variable type, indexed by the
// FST code. An empty entry has no VCD keyword of its own.
var fstVarTypes = map[byte]string{
	C.FST_VT_VCD_EVENT:          "event",
	C.FST_VT_VCD_INTEGER:        "integer",
	C.FST_VT_VCD_PARAMETER:      "parameter",
	C.FST_VT_VCD_REAL:           "real",
	C.FST_VT_VCD_REAL_PARAMETER: "real",
	C.FST_VT_VCD_REG:            "reg",
	C.FST_VT_VCD_SUPPLY0:        "supply0",
	C.FST_VT_VCD_SUPPLY1:        "supply1",
	C.FST_VT_VCD_TIME:           "time",
	C.FST_VT_VCD_TRI:            "tri",
	C.FST_VT_VCD_TRIAND:         "triand",
	C.FST_VT_VCD_TRIOR:          "trior",
	C.FST_VT_VCD_TRIREG:         "trireg",
	C.FST_VT_VCD_TRI0:           "tri0",
	C.FST_VT_VCD_TRI1:           "tri1",
	C.FST_VT_VCD_WAND:           "wand",
	C.FST_VT_VCD_WIRE:           "wire",
	C.FST_VT_VCD_WOR:            "wor",
	C.FST_VT_VCD_PORT:           "wire",
	C.FST_VT_VCD_REALTIME:       "real",
	C.FST_VT_GEN_STRING:         "string",
	C.FST_VT_SV_SHORTREAL:       "real",
}

// varType is the VCD keyword for an FST variable type. A type this table
// does not name becomes `wire`, which is how a VCD writer spells anything
// it has no better word for, and which the parser reads back as a wire.
func varType(t byte) string {
	if s, ok := fstVarTypes[t]; ok {
		return s
	}
	return "wire"
}
