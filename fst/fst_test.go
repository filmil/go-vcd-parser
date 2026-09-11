// SPDX-License-Identifier: Apache-2.0

package fst

import (
	"testing"

	"github.com/filmil/go-vcd-parser/vcd"
)

func TestIdCode(t *testing.T) {
	// The codes have to be distinct, and stable, because they are what
	// joins a value row to its signal row in the database.
	seen := map[string]uint32{}
	for h := uint32(0); h < 5000; h++ {
		c := idCode(h)
		if c == "" {
			t.Fatalf("idCode(%d) is empty", h)
		}
		for i := 0; i < len(c); i++ {
			if c[i] < '!' || c[i] > '~' {
				t.Fatalf("idCode(%d) = %q has a character outside printable ASCII", h, c)
			}
		}
		if prev, ok := seen[c]; ok {
			t.Fatalf("idCode(%d) = idCode(%d) = %q", h, prev, c)
		}
		seen[c] = h
	}
}

func TestSplitIndices(t *testing.T) {
	ix := func(n int) *int { return &n }
	for _, tt := range []struct {
		in      string
		name    string
		indices []*vcd.IdxT
	}{
		{"clk", "clk", nil},
		{"data [7:0]", "data", []*vcd.IdxT{{MsbIndex: ix(7), LsbIndex: ix(0)}}},
		{"q [3]", "q", []*vcd.IdxT{{Index: ix(3)}}},
		// A name that only looks like it has a range keeps its brackets.
		{"weird [a:b]", "weird [a:b]", nil},
		{"no_bracket [7:0", "no_bracket [7:0", nil},
		{"trailing]", "trailing]", nil},
	} {
		name, indices := splitIndices(tt.in)
		if name != tt.name {
			t.Errorf("splitIndices(%q) name = %q, want %q", tt.in, name, tt.name)
		}
		if len(indices) != len(tt.indices) {
			t.Fatalf("splitIndices(%q) got %d indices, want %d", tt.in, len(indices), len(tt.indices))
		}
		for i := range indices {
			if got, want := indices[i].AsString(), tt.indices[i].AsString(); got != want {
				t.Errorf("splitIndices(%q)[%d] = %v, want %v", tt.in, i, got, want)
			}
		}
	}
}

func TestTimeUnit(t *testing.T) {
	// The pair has to reproduce the exponent it came from: the unit's own
	// exponent plus the digits left over in the number.
	for exp := -15; exp <= 0; exp++ {
		u, ue := timeUnit(exp)
		if u == nil {
			t.Fatalf("timeUnit(%d) has no unit", exp)
		}
		n := 1.0
		for i := 0; i < exp-ue; i++ {
			n *= 10
		}
		ts := vcd.TimescaleT{Number: int64(n), Unit: u}
		want := pow10(exp)
		if got := ts.AsSeconds(); !closeEnough(got, want) {
			t.Errorf("timeUnit(%d) spells %v s, want %v s", exp, got, want)
		}
	}
	if u, _ := timeUnit(-16); u != nil {
		t.Errorf("timeUnit(-16) should have no VCD spelling, got %+v", u)
	}
}

func pow10(e int) float64 {
	r := 1.0
	for ; e < 0; e++ {
		r /= 10
	}
	for ; e > 0; e-- {
		r *= 10
	}
	return r
}

// closeEnough compares two times that were built by different routes
// through binary floating point.
func closeEnough(a, b float64) bool {
	if a == b {
		return true
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= b/1e9
}

func TestVarType(t *testing.T) {
	// Every keyword this maps to must be one the VCD parser knows, or the
	// signal lands in the database as an unknown kind.
	for code := 0; code < 40; code++ {
		got := varType(byte(code))
		v := vcd.VarT{VarType: got}
		if v.GetVarKind() == vcd.VarKindUnknown {
			t.Errorf("varType(%d) = %q, which the VCD parser does not know", code, got)
		}
	}
}

func TestScopeKind(t *testing.T) {
	for code := 0; code < 12; code++ {
		// Kind() panics or misreports if no field is set, so every FST
		// scope type has to select exactly one.
		k := scopeKind(byte(code))
		n := 0
		for _, b := range []bool{k.Begin, k.Fork, k.Function, k.Module, k.Task} {
			if b {
				n++
			}
		}
		if n != 1 {
			t.Errorf("scopeKind(%d) set %d fields, want exactly 1", code, n)
		}
	}
}
