package vcd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// basicParseInputs is shared with the differential test in
// stream_diff_test.go, so both parsers see the same corpus.
var basicParseInputs = []string{
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

func TestBasicParse(t *testing.T) {
	t.Parallel()
	for i, test := range basicParseInputs {
		test := test
		t.Run(fmt.Sprintf("rule %v", i), func(t *testing.T) {
			parser := NewParser[File]()
			r := strings.NewReader(test)
			if _, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r); err != nil {
				t.Errorf("parse error:\n\t\t`%v`\n\t\t\t%v", test, err)
			}

		})
	}
}

// TestVarParse tests the special treatment of the `$var` directive.
//
// $var is special because the short id code is a very unusual token.
func TestVarParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
	}{
		{input: `$var logic 8 :! fifo_memory[48][7:0] $end`},
	}
	parser := NewParser[File]()
	for i, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			r := strings.NewReader(test.input)
			_, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Errorf("parse error: input:`%+v`: %+v", test.input, err)
			}
		})
	}
}

func TestBitParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
	}{
		{"0V#"},
		{"0V"},
		{"0VAB"},
		{"0#"},
		{"0##"},
		{"0###"},
		{"0VAB###"},
		{"0###VAB"},
	}
	parser := NewIdParser[ScalarValueChangeT]()
	for i, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			r := strings.NewReader(test.input)
			_, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Errorf("parse error: input:`%+v`: %+v", test.input, err)
			}
		})
	}
}

func TestScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected ScopeT
	}{
		{"$scope module top $end", ScopeT{
			Scope:     true,
			ScopeKind: ScopeKindT{Module: true},
			Id:        "top",
			KwEnd:     true,
		}},
	}
	parser := NewParser[ScopeT]()
	for i, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			r := strings.NewReader(test.input)
			actual, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Fatalf("parse error: %+v: %v", test.input, err)
			}
			if !reflect.DeepEqual(&test.expected, actual) {
				t.Errorf("\nwant: %+v\ngot:  %+v", &test.expected, actual)
			}
		})
	}
}

func TestTimescale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected TimescaleT
	}{
		{"$timescale 10ns $end", TimescaleT{
			Kw:     true,
			Number: 10,
			Unit: &TimeUnit{
				NanoSecond: true,
			},
			Kw2: true,
		}},
	}
	parser := NewParser[TimescaleT]()
	for i, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			r := strings.NewReader(test.input)
			actual, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Fatalf("parse error: %+v: %v", test.input, err)
			}
			if !reflect.DeepEqual(&test.expected, actual) {
				t.Errorf("\nwant: %+v\ngot:  %+v", spew.Sdump(&test.expected), spew.Sdump(actual))
			}
		})
	}
}

func Ptr[T any](v T) *T {
	return &v
}

func TestResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected File
	}{
		{input: `
            $enddefinitions $end
            srx_get_start_bit ^
        `,
			expected: File{
				DeclarationCommand: []*DeclarationCommandT{
					{
						EndDefinitions: Ptr(true),
					},
				},
				SimulationCommand: []*SimulationCommandT{
					{
						ValueChange: &ValueChangeT{
							VectorValueChange: &VectorValueChangeT{
								VectorValueChange2: &VectorValueChange2T{
									Value:  "srx_get_start_bit",
									IdCode: "^",
								},
							},
						},
					}, //
				},
			},
		},
		{input: `
            $enddefinitions $end
            bUUUUUUUU F
        `,
			expected: File{
				DeclarationCommand: []*DeclarationCommandT{
					{
						EndDefinitions: Ptr(true),
					},
				},
				SimulationCommand: []*SimulationCommandT{
					{
						ValueChange: &ValueChangeT{
							VectorValueChange: &VectorValueChangeT{
								VectorValueChange1: &VectorValueChange1T{
									Value:  "bUUUUUUUU",
									IdCode: "F",
								},
							},
						},
					}, //
				},
			},
		},
	}
	parser := NewParser[File]()
	for i, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			r := strings.NewReader(test.input)
			actual, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Fatalf("parse error: %+v: %v", test.input, err)
			}
			if !reflect.DeepEqual(&test.expected, actual) {
				t.Errorf("\nwant: %v\ngot:  %v", spew.Sdump(&test.expected), spew.Sdump(actual))
			}
		})
	}
}

// TestIdCodeLikeAToken covers identifier codes that look like another
// token. A VCD writer may use any printable ASCII for a code, so a
// code can spell a timestamp, `#0`, a real, `R0`, a state string,
// `s1`, or a binary value, `b0`. What follows a value is the code.
// idCodeLikeATokenInputs is shared with the differential test.
var idCodeLikeATokenInputs = []struct {
	input string
	value string
	code  string
}{
		{"b0 #0", "b0", "#0"},
		{"bx #1", "bx", "#1"},
		{"b0 R0", "b0", "R0"},
		{"b1 r1.5", "b1", "r1.5"},
		{"b0 s1", "b0", "s1"},
		{"b0 b0", "b0", "b0"},
	{"b0 #", "b0", "#"},
}

func TestIdCodeLikeAToken(t *testing.T) {
	t.Parallel()
	parser := NewParser[File]()
	for i, test := range idCodeLikeATokenInputs {
		test := test
		t.Run(test.input, func(t *testing.T) {
			r := strings.NewReader("$enddefinitions $end\n" + test.input + "\n")
			f, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
			if err != nil {
				t.Fatalf("parse error: %+v: %v", test.input, err)
			}
			if len(f.SimulationCommand) != 1 || f.SimulationCommand[0].ValueChange == nil {
				t.Fatalf("%v: no value change: %v", test.input, spew.Sdump(f))
			}
			vc := f.SimulationCommand[0].ValueChange
			if got := "b" + vc.GetValue(); got != test.value {
				t.Errorf("%v: value %v, want %v", test.input, got, test.value)
			}
			if got := vc.GetIdCode(); got != test.code {
				t.Errorf("%v: code %v, want %v", test.input, got, test.code)
			}
		})
	}
}

//type VarTWrap struct {
//V *VarT `parser:"@@"`
//}

//func TestVar(t *testing.T) {
//t.Parallel()
//tests := []struct {
//input   string
//checkFn func(t *testing.T, v *VarT)
//}{
//{
//input: `$var string 0 & uart_rx_state $end`,
//checkFn: func(t *testing.T, v *VarT) {
//if v.Id.Name != "uart_rx_state" {
//t.Errorf("Name: %v", v.Id.Name)
//}
//},
//},
//}
//parser := NewParser[VarTWrap]()
//for i, test := range tests {
//test := test
//t.Run(test.input, func(t *testing.T) {
//r := strings.NewReader(test.input)
//actual, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r)
//if err != nil {
//t.Fatalf("parse error: %+v: %v", test.input, err)
//}
//test.checkFn(t, actual.V)
//})
//}
//}
