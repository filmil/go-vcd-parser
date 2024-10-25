package vcd

import (
	"fmt"
	"strings"
	"testing"
)

// TestParseFromTheWild tests stanzas found in realistic VCD files.
func TestParseFromTheWild(t *testing.T) {
	t.Parallel()
	tests := []string{
		// A file produced by VNC, I think.
		`$attrbegin misc 02 STD_LOGIC 1030 $end`,
		`$var integer 1 0 write_en $end`,
		`$var integer 1 : write_en $end`,
		`$var integer 1 K write_en $end`,
		`$var logic 1 *K write_en $end`,
		`$var string 0 C bus_is_read $end`,
		`$var logic 1 [ uart_tx_data $end`,
		`$var logic 8 h fifo_memory[0][7:0] $end`,
		`$var logic 8 0! fifo_memory[38][7:0] $end`,
		`$var logic 8 :! fifo_memory[48][7:0] $end`,
		`$attrend $end`,
		`
         $enddefinitions $end
         $dumpvars 0V# $end`,
	}

	for i, test := range tests {
		test := test
		t.Run(fmt.Sprintf("rule %v", i), func(t *testing.T) {
			parser := NewParser[VCDFile]()
			r := strings.NewReader(test)
			if _, err := parser.Parse(fmt.Sprintf("(rule %v)", i), r); err != nil {
				t.Errorf("parse error: `%v`: %+v", test, err)
			}

		})
	}
}
