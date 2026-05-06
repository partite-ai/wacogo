package wasi_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/wasi"
)

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
