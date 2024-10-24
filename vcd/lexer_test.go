package vcd

import (
	"fmt"
	"regexp"
	"testing"
)

func TestBinstring(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern string
		yes, no []string
	}{
		{
			pattern: BinstringPattern,
			yes:     []string{"b10", "bzZ10z01"},
			no:      []string{"", "b|"},
		},
		{
			pattern: RealStringPattern,
			yes:     []string{"r10.0", "R10.0", "r-10.0"},
			no:      []string{"", "q10.0"},
		},
		{
			pattern: FloatPattern,
			yes:     []string{"1.0", "1", "0", "-1", "-1e90"},
			no:      []string{""},
		},
		{
			pattern: IntPattern,
			yes:     []string{"1", "0", "-42"},
			no:      []string{"", "1.0", "hello"},
		},
		{
			pattern: StringPattern,
			yes:     []string{"Hello"},
			no:      []string{"Hello world"},
		},
		{
			pattern: TimestampPattern,
			yes:     []string{"#0", "#424242"},
			no:      []string{"", "$ 0", "$ 0", "#-42", "# 42"},
		},
		{
			pattern: IdentifierPattern,
			yes:     []string{
                "a", "x", "y", "_", "a0",
                "ooGa_BOoga", "__many_underscores",
                "__1", "_something23b",
            },
			no:      []string{"", "$ 0", "$ 0", "#-42", "# 42", "0abc"},
		},
		{
			pattern: AnyWordPattern,
			yes:     []string{
                "a", "x", "y", "_", "a0",
                "VERILOG-SIMULATOR",
            },
			no:      []string{""},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(fmt.Sprintf("pattern:%v", test.pattern), func(t *testing.T) {
			m := MatchEntire(test.pattern)
			r := regexp.MustCompile(MatchEntire(test.pattern))
			for _, y := range test.yes {
				ye := AddEOL(y)
				if !r.MatchString(ye) {
					t.Errorf("Experssion `%v` should match `%v` but does not", y, m)
				}
			}
			for _, n := range test.no {
				ne := AddEOL(n)
				if r.MatchString(ne) {
					t.Errorf("Experssion `%v` should NOT match `%v` but does", n, m)
				}
			}
		})
	}
}

func MatchEntire(p string) string {
	return fmt.Sprintf("^%sXXXEOL", p)
}

func AddEOL(p string) string {
	return fmt.Sprintf("%sXXXEOL", p)
}
