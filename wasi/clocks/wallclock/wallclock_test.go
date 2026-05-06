package wallclock

import (
	"context"
	"testing"
	"time"
)

var bgCtx = context.Background()

func TestNowReportsInjectedTime(t *testing.T) {
	fixed := time.Date(2024, 6, 1, 12, 34, 56, 789_000_000, time.UTC)
	c := &clock{now: func() time.Time { return fixed }}

	got, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	wantSec := uint64(fixed.Unix())
	wantNsec := uint32(789_000_000)
	if got.Seconds != wantSec || got.Nanoseconds != wantNsec {
		t.Fatalf("Now = {%d, %d}, want {%d, %d}", got.Seconds, got.Nanoseconds, wantSec, wantNsec)
	}
}

func TestNowSubSecondNanosecondsPreserved(t *testing.T) {
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 123_456_789, time.UTC)
	c := &clock{now: func() time.Time { return fixed }}

	got, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	if got.Nanoseconds != 123_456_789 {
		t.Fatalf("Nanoseconds = %d, want 123456789", got.Nanoseconds)
	}
	if got.Nanoseconds >= 1_000_000_000 {
		t.Fatalf("Nanoseconds %d must be < 1e9 per WIT spec", got.Nanoseconds)
	}
}

func TestNowAdvancesBetweenCalls(t *testing.T) {
	calls := 0
	c := &clock{now: func() time.Time {
		calls++
		return time.Unix(int64(1700000000+calls), 0)
	}}

	first, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	second, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	if second.Seconds <= first.Seconds {
		t.Fatalf("second Now (%d) did not advance past first (%d)", second.Seconds, first.Seconds)
	}
}

func TestNowFromHostTimeIsRecent(t *testing.T) {
	c := newClock()

	before := time.Now().Unix()
	got, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	after := time.Now().Unix()

	if int64(got.Seconds) < before-1 || int64(got.Seconds) > after+1 {
		t.Fatalf("Now seconds = %d, expected within [%d, %d]", got.Seconds, before, after)
	}
	if got.Nanoseconds >= 1_000_000_000 {
		t.Fatalf("Nanoseconds %d must be < 1e9", got.Nanoseconds)
	}
}

func TestPreEpochTimeClampsToZero(t *testing.T) {
	preEpoch := time.Date(1969, 6, 1, 0, 0, 0, 0, time.UTC)
	c := &clock{now: func() time.Time { return preEpoch }}

	got, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	if got.Seconds != 0 || got.Nanoseconds != 0 {
		t.Fatalf("pre-epoch Now = {%d, %d}, want {0, 0}", got.Seconds, got.Nanoseconds)
	}
}

func TestEpochInstantIsZero(t *testing.T) {
	c := &clock{now: func() time.Time { return time.Unix(0, 0).UTC() }}

	got, err := c.Now(bgCtx)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	if got.Seconds != 0 || got.Nanoseconds != 0 {
		t.Fatalf("epoch Now = {%d, %d}, want {0, 0}", got.Seconds, got.Nanoseconds)
	}
}

func TestResolutionReportsOneNanosecond(t *testing.T) {
	c := newClock()
	got, err := c.Resolution(bgCtx)
	if err != nil {
		t.Fatalf("Resolution: %v", err)
	}
	if got.Seconds != 0 || got.Nanoseconds != 1 {
		t.Fatalf("Resolution = {%d, %d}, want {0, 1}", got.Seconds, got.Nanoseconds)
	}
}

func TestResolutionNanosecondsBelowOneSecond(t *testing.T) {
	c := newClock()
	got, err := c.Resolution(bgCtx)
	if err != nil {
		t.Fatalf("Resolution: %v", err)
	}
	if got.Nanoseconds >= 1_000_000_000 {
		t.Fatalf("Resolution Nanoseconds = %d, must be < 1e9 per WIT spec", got.Nanoseconds)
	}
}
