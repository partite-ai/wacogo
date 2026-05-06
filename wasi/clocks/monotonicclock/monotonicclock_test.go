package monotonicclock

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/partite-ai/wacogo/wasi/io/poll"
)

var bgCtx = context.Background()

func pollableReady(t *testing.T, ph *poll.PollableHandle) bool {
	t.Helper()
	r, err := ph.Ready(bgCtx)
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	return r
}

func TestNowAdvancesWithVirtualTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		start, _ := c.Now(nil)
		time.Sleep(2*time.Second + 500*time.Millisecond)
		got, err := c.Now(bgCtx)
		if err != nil {
			t.Fatalf("Now: %v", err)
		}
		want := uint64(2*time.Second + 500*time.Millisecond)
		if got-start != want {
			t.Fatalf("Now = %d, want %d", got, want)
		}
	})
}

func TestResolutionReturnsOneNanosecond(t *testing.T) {
	c := newClock(nil)
	got, err := c.Resolution(bgCtx)
	if err != nil {
		t.Fatalf("Resolution: %v", err)
	}
	if got != 1 {
		t.Fatalf("Resolution = %d, want 1", got)
	}
}

func TestSubscribeDurationNotReadyBeforeDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		ph, err := c.SubscribeDuration(bgCtx, uint64(time.Second))
		if err != nil {
			t.Fatalf("SubscribeDuration: %v", err)
		}
		if pollableReady(t, ph) {
			t.Fatal("pollable should not be ready before duration elapses")
		}
		time.Sleep(500 * time.Millisecond)
		if pollableReady(t, ph) {
			t.Fatal("pollable should not be ready halfway through duration")
		}
	})
}

func TestSubscribeDurationReadyAfterElapsed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		ph, err := c.SubscribeDuration(bgCtx, uint64(time.Second))
		if err != nil {
			t.Fatalf("SubscribeDuration: %v", err)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if !pollableReady(t, ph) {
			t.Fatal("pollable should be ready when duration has elapsed")
		}
	})
}

func TestSubscribeDurationZeroIsImmediatelyReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		ph, err := c.SubscribeDuration(bgCtx, 0)
		if err != nil {
			t.Fatalf("SubscribeDuration: %v", err)
		}
		if !pollableReady(t, ph) {
			t.Fatal("zero-duration pollable should be ready immediately")
		}
	})
}

func TestSubscribeInstantInPastIsImmediatelyReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		time.Sleep(time.Second)
		ph, err := c.SubscribeInstant(bgCtx, uint64(500*time.Millisecond))
		if err != nil {
			t.Fatalf("SubscribeInstant: %v", err)
		}
		if !pollableReady(t, ph) {
			t.Fatal("past-instant pollable should be ready immediately")
		}
	})
}

func TestSubscribeInstantInFutureBecomesReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		start, _ := c.Now(nil)
		ph, err := c.SubscribeInstant(bgCtx, start+uint64(2*time.Second))
		if err != nil {
			t.Fatalf("SubscribeInstant: %v", err)
		}
		if pollableReady(t, ph) {
			t.Fatal("future-instant pollable should not be ready yet")
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if !pollableReady(t, ph) {
			t.Fatal("pollable should be ready once the instant is reached")
		}
	})
}

func TestBlockReturnsImmediatelyWhenReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		ph, err := c.SubscribeDuration(bgCtx, 0)
		if err != nil {
			t.Fatalf("SubscribeDuration: %v", err)
		}
		if err := ph.Block(bgCtx); err != nil {
			t.Fatalf("Block: %v", err)
		}
	})
}

func TestBlockWaitsForDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		ph, err := c.SubscribeDuration(bgCtx, uint64(time.Second))
		if err != nil {
			t.Fatalf("SubscribeDuration: %v", err)
		}

		done := make(chan error, 1)
		go func() { done <- ph.Block(bgCtx) }()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("Block returned before deadline")
		default:
		}

		time.Sleep(time.Second)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Block: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Block did not return after deadline")
		}
	})
}

func TestBlockHonorsContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClock(nil)
		ph, err := c.SubscribeDuration(bgCtx, uint64(time.Hour))
		if err != nil {
			t.Fatalf("SubscribeDuration: %v", err)
		}

		ctx, cancel := context.WithCancel(bgCtx)
		done := make(chan error, 1)
		go func() { done <- ph.Block(ctx) }()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("Block returned before cancel")
		default:
		}

		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Block err = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Block did not return after cancel")
		}
	})
}
