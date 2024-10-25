package vcd

import (
	"fmt"
	"strings"
	"testing"
)

func TestBasicParse(t *testing.T) {
	t.Parallel()
	tests := []string{
		"",
		`$date something else $end`,
		`$comment  this is some comment "string" $end`,
		`$comment  this is some
           comment "string" $end`,
		`$enddefinitions $end`,
		`
            $comment this is an illustration of $enddefinitions $end
            $enddefinitions $end
        `,
		`$scope begin some_id $end`,
		`$scope
            module top
         $end`,

		`
        $timescale 10 ns $end
        `,
		`$timescale 10ps $end`,
		`$timescale 10s $end`,

		`$upscope $end`,

		// 18.2.3.7
		`$version
            VERILOG-SIMULATOR 1.0a
            $dumpfile("dump1.dump")
        $end`,

		// 18.2.3.8
		`$var
            integer 32 (2 index
         $end
        `,
		`$var
            integer 32 (2 index[ 6 ]
         $end
        `,
		`$var
            integer 32 (2 index[6]
         $end
        `,
		`$var
        integer 32 (2 index[ 6 : 10 ]
         $end
        `,
		`$var
        integer 32 (2 index[6:10 ]
         $end
        `,
		`
$var reg 32 (k accumulator[31:0] $end
        `,
		// 18.2.3.9
		`
        $dumpall 1*@ x*# 0*$ bx (k $end
        `,

		// 18.2.3.10
		`$dumpoff 1*@ x*# 0*$ bx (k $end`,

		// 18.2.3.11
		`$dumpon 1*@ x*# 0*$ bx (k $end`,

		// 18.2.3.12
		`$dumpvars x*# z*$ b0 (k $end`,
	}

	for i, test := range tests {
		test := test
		t.Run(fmt.Sprintf("rule %v", i), func(t *testing.T) {
			parser := NewParser()
			r := strings.NewReader(test)
			if _, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r); err != nil {
				t.Errorf("parse error: `%v`: %+v", test, err)
			}

		})
	}
}

// TestVarParse tests the special treatment of the `$var` directive.
//
// $var is special because the short id code is a very unusual token.
func TestVarParse(t *testing.T) {
	tests := []struct {
		input string
	}{
		{input: `$var logic 8 :! fifo_memory[48][7:0] $end`},
	}
	for i, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			parser := NewParser()
			r := strings.NewReader(test.input)
			_, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Errorf("parse error: input:`%+v`: %+v", test.input, err)
			}
		})
	}
}
