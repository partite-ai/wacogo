package host

import (
	"context"
	"sync"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

// TestInstantiate_ParallelSameComponent guards against race conditions
// in the per-Component arena. Run with `go test -race`.
func TestInstantiate_ParallelSameComponent(t *testing.T) {
	ctx := context.Background()
	engine := core.NewEngine(ctx)
	defer engine.Close(ctx)

	b := NewBuilder(engine, "test:parallel")
	noop := func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, _ []uint64) error {
		return nil
	}
	b.AddFunction("noop", &FuncType{}, noop)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	const N = 16
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst, err := comp.Instantiate(ctx)
			if err != nil {
				errs <- err
				return
			}
			_ = inst.Close(ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("parallel Instantiate: %v", err)
		}
	}
}
