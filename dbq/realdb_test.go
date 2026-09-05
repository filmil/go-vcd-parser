package dbq

import (
	"testing"
	"time"
)

// TestRealDumpTimesAreRight reads the database the build makes from
// vcd/files/samples/tb.vcd, whose $timescale is 1fs. Before the timescale
// was read from Meta, every duration here came out a thousand times too
// large: reset released at "10us" rather than 10ns.
func TestRealDumpTimesAreRight(t *testing.T) {
	dbx, _, err := GetTestDB()
	if err != nil {
		t.Fatalf("no test database: %v", err)
	}
	q := New(dbx)

	release := q.Signal("//wb_uart_tb/reset").FindFirst("0")
	if err := release.IsOk(); err != nil {
		t.Fatalf("reset never released: %v", err)
	}
	if got, want := release.T(), uint64(10_000_000); got != want {
		t.Errorf("reset releases at %v ticks, want %v", got, want)
	}
	if got, want := release.D(), 10*time.Nanosecond; got != want {
		t.Errorf("reset releases at %v, want %v", got, want)
	}

	// The clock runs at 200 MHz, so consecutive rising edges are 5 ns
	// apart. IsClock checks the half period either side of a falling
	// edge, which is what makes this a real check and not a spot value.
	clk := q.Signal("//wb_uart_tb/clk")
	if err := IsClock(&TimestampZero, clk, 200e6); err != nil {
		t.Errorf("clk is not a 200 MHz clock: %v", err)
	}
}
