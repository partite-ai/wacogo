package wasi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/partite-ai/wacogo"
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
