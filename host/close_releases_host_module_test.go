package host_test

import (
	"context"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// TestClose_ReleasesInstance verifies that a closed instance becomes
// unreachable while its component and engine stay open, so instances
// created and closed over the engine's lifetime do not accumulate.
func TestClose_ReleasesInstance(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("release-host")
	b.AddFunction("noop", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	instantiateAndClose := func() *host.ComponentInstance {
		inst, err := comp.Instantiate(ctx)
		if err != nil {
			t.Fatalf("Instantiate: %v", err)
		}
		if err := inst.Close(ctx); err != nil {
			t.Fatalf("Close: %v", err)
		}
		return inst
	}

	first := weak.Make(instantiateAndClose())
	// wazero may keep a reference to the most recent instance, so the
	// first one is checked after later instances replace it.
	for range 3 {
		instantiateAndClose()
	}

	// Objects reachable from wazero's finalizers survive until those
	// finalizers run, so collect a few times before deciding.
	for i := 0; i < 20 && first.Value() != nil; i++ {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if first.Value() != nil {
		t.Fatal("closed instance is still reachable while its engine is open")
	}
}
