package dbq

import (
	"fmt"
	"testing"
	"time"

	u "github.com/dsnet/golib/unitconv"
)

func (self Timestamp) Testify(t *testing.T) *Timestamp {
	t.Helper()
	if self.Error() != nil {
		t.Fatalf("lookup error:\n\t%v", self.name, self.Error())
		return nil
	}
	if self.IsNone() {
		t.Errorf("timestamp is None for signal: %q", self.name)
		return nil
	}
	return &self
}

func Testify00(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("Testify00 found error:\n\t%v", err)
	}
}

func Testify01[V any](t *testing.T, f func() (V, error)) V {
	t.Helper()
	v, err := f()
	if err != nil {
		t.Errorf("Testify01 found error:\n\t%v", err)
	}
	return v
}

func Diff(sooner, later *Timestamp) time.Duration {
	d := later.D() - sooner.D()
	return d
}

func (self Timestamp) After(other *Timestamp) bool {
	return self.D()-other.D() > 0
}

func (self Timestamp) Before(other *Timestamp) bool {
	return !self.Eq(other.T()) && !self.After(other)
}

func IsDuration(dur time.Duration, hp time.Duration) (time.Duration, error) {
	early := hp - 1*time.Nanosecond
	late := hp - 2*time.Nanosecond
	if dur < hp-1*time.Nanosecond || dur > hp+1*time.Nanosecond {
		return 0 * time.Nanosecond,
			fmt.Errorf("expected difference: %v < %v < %v,"+
				" but got difference: %v", early, dur, late, dur)
	}
	return dur, nil
}

// Check that the duration between two timestamps is about the given duration.
func IsDurationApprox(sooner, later *Timestamp, d time.Duration) error {
	if sooner.After(later) {
		return fmt.Errorf("duration mismatch:\n\tduration %v on signal %q should be sooner than %v on signal %q", sooner.D(), sooner.name, later.D(), later.name)
	}
	ds := sooner.D()
	dl := later.D()
	res, err := IsDuration(dl-ds, d)
	if err != nil {
		return fmt.Errorf("duration %v mismatch between:\n\t(1) timestamp %v on signal %q\n\t(2) timestamp %v on siqnal %q:\n\tdoes not fulfill requested duration: %v\n\t%w",
			res, sooner.D(), sooner.name, later.D(), later.name, d, err)
	}

	return nil
}

func IsClock(from *Timestamp, clk *Signal, freq float64) error {
	periodSec := 1.0 / freq
	periodNs := int64(periodSec * 1e+9)
	p := time.Duration(periodNs) * time.Nanosecond
	hp := p / 2
	fmhz := u.FormatPrefix(float64(freq), u.SI, 2)

	cts1 := clk.FindAfter(from, "1")
	if cts1.Error() != nil {
		return fmt.Errorf(
			"check signal %v has frequency %vHz:\n\t"+
				"value '1' on %q could not be found after %v:\n\t",
			clk,
			fmhz,
			from.name, from.D(), cts1.Error(),
		)
	}
	if cts1.IsNone() {
		return fmt.Errorf(
			"check signal %v has frequency %vHz:\n\t"+
				"value '1' on %q could not be found after %v",
			clk,
			fmhz,
			from.name, from.D())
	}
	cts2 := clk.FindAfter(cts1, "0")
	if err := cts2.IsOk(); err != nil {
		return fmt.Errorf("qq: %w", err)
	}
	cts3 := clk.FindAfter(cts2, "1")
	if err := cts3.IsOk(); err != nil {
		return fmt.Errorf(
			"signal %q has frequency %vHz:\n\t"+
				"\n\t%w", clk,
			fmhz,
			err)
	}

	d1 := Diff(cts1, cts2)
	_, err := IsDuration(d1, hp)
	if err != nil {
		return fmt.Errorf(
			"signal %q has frequency %vHz:\n\t"+
				"unexpected difference between rising and falling edge:\n\t"+
				"between timestamps:\n\t(1) %v and\n\t(2) %v:\n\t%w",
			clk,
			fmhz,
			cts1, cts2, err)
	}
	d2 := Diff(cts2, cts3)
	_, err = IsDuration(d2, hp)
	if err != nil {
		return fmt.Errorf("2:\n\t%w", err)
	}
	return nil
}

func (self Timestamp) IsOk() error {
	if self.Error() != nil {
		return fmt.Errorf("timestamp on signal %q is not OK:\n\t%w",
			self.name, self.Error())
	}
	if self.IsNone() {
		return fmt.Errorf("value on %q could not be found", self.name)
	}
	return nil
}
