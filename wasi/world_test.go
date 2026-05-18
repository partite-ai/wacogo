package wasi_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi"
)

// Compile-time assertion: *http.Client satisfies wasi.HTTPDoer.
var _ wasi.HTTPDoer = (*http.Client)(nil)

func TestNewWorld(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	t.Cleanup(func() {
		if err := e.Close(ctx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	var cfg wasi.Config
	w, err := wasi.NewWorld(ctx, e, &cfg)
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	if err := w.Close(ctx); err != nil {
		t.Fatalf("World.Close: %v", err)
	}
}

type recordingDoer struct {
	called bool
}

func (d *recordingDoer) Do(r *http.Request) (*http.Response, error) {
	d.called = true
	return nil, errors.New("recordingDoer never issues real requests")
}

type countingListener struct {
	before atomic.Int64
	after  atomic.Int64
	last   atomic.Value // string: name of the most recent call
}

func (l *countingListener) BeforeCall(_ context.Context, _ *host.ComponentInstance, _ host.CallKind, name string, _ []uint64) {
	l.before.Add(1)
	l.last.Store(name)
}

func (l *countingListener) AfterCall(_ context.Context, _ *host.ComponentInstance, _ host.CallKind, _ string, _ []uint64, _ error) {
	l.after.Add(1)
}

func TestNewWorld_CallListenerObservesWasiCalls(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	t.Cleanup(func() {
		if err := e.Close(ctx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	rec := &countingListener{}
	cfg := wasi.Config{CallListener: rec}
	w, err := wasi.NewWorld(ctx, e, &cfg)
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	defer w.Close(ctx)

	fn := w.WallClock.Core().ExportedFunc("now")
	if fn == nil {
		t.Fatal("wall-clock missing 'now' export")
	}
	if _, err := fn.Call(ctx); err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got := rec.before.Load(); got == 0 {
		t.Fatal("listener never observed a wasi call (before)")
	}
	if got, want := rec.after.Load(), rec.before.Load(); got != want {
		t.Fatalf("before/after mismatch: before=%d after=%d", want, got)
	}
	if name, _ := rec.last.Load().(string); name != "now" {
		t.Fatalf("last call name = %q, want %q", name, "now")
	}
}

func TestNewWorld_AcceptsCustomDoer(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	t.Cleanup(func() {
		if err := e.Close(ctx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	doer := &recordingDoer{}
	cfg := wasi.Config{HttpClient: doer}
	w, err := wasi.NewWorld(ctx, e, &cfg)
	if err != nil {
		t.Fatalf("NewWorld with custom Doer: %v", err)
	}
	if err := w.Close(ctx); err != nil {
		t.Fatalf("World.Close: %v", err)
	}
}
