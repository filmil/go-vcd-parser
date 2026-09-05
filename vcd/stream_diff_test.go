package vcd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// assertSameAsParticiple parses input with both the grammar-driven parser
// and the streaming parser and requires that they agree.
func assertSameAsParticiple(t *testing.T, name, input string) {
	t.Helper()
	want, err := NewParser[File]().Parse(name, strings.NewReader(input))
	if err != nil {
		t.Fatalf("%v: participle parse error: %v", name, err)
	}
	got, err := ParseFile(name, strings.NewReader(input))
	if err != nil {
		t.Fatalf("%v: streaming parse error: %v", name, err)
	}
	if !reflect.DeepEqual(NormalizeForCompare(want), NormalizeForCompare(got)) {
		t.Errorf("%v:\nwant: %v\ngot:  %v",
			name, spew.Sdump(want), spew.Sdump(got))
	}
}

// TestStreamMatchesParticiple runs every snippet the grammar-driven parser
// is tested with through the streaming parser too.
func TestStreamMatchesParticiple(t *testing.T) {
	t.Parallel()
	for i, input := range basicParseInputs {
		t.Run(fmt.Sprintf("basic %v", i), func(t *testing.T) {
			assertSameAsParticiple(t, fmt.Sprintf("(basic %v)", i), input)
		})
	}
	for i, tc := range idCodeLikeATokenInputs {
		t.Run(fmt.Sprintf("idcode %v", i), func(t *testing.T) {
			assertSameAsParticiple(t, fmt.Sprintf("(idcode %v)", i),
				"$enddefinitions $end\n"+tc.input+"\n")
		})
	}
}
