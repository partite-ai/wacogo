package core

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"
)

// TestLoadComponent_ParallelDistinctArenas guards against race
// conditions in the per-load TypeArena. Run with `go test -race`.
func TestLoadComponent_ParallelDistinctArenas(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	bin, err := os.ReadFile("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	const N = 16
	var wg sync.WaitGroup
	comps := make([]*Component, N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
			if err != nil {
				errs <- err
				return
			}
			comps[i] = c
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("parallel LoadComponent: %v", err)
		}
	}
	for i, c := range comps {
		if c == nil || c.wpType == nil {
			t.Fatalf("comp[%d] missing wpType", i)
		}
	}
}
