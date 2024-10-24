package vcd

import (
	"fmt"
	"strings"
	"testing"
)

func TestParses(t *testing.T) {
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
        integer 32 (2 index[ 6 : 10 ]
         $end
        `,
		// 18.2.3.9
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
