package host_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// TestDirect_CompToHostFailures checks that a host function called from
// wasm - which runs directly in Go, not through the stub module - fails
// the call the way a wazero host function would: a returned error and a
// panic both come back as the call's error, and a Go runtime panic keeps
// its stack trace.
func TestDirect_CompToHostFailures(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	wantErr := errors.New("provider failed")
	hb := e.NewHostBuilder("provider-host")
	hb.AddFunction("provider", &host.FuncType{
		Params:  []host.Param{{Name: "x", Type: host.S32}},
		Results: []host.ResultDecl{{Name: "", Type: host.S32}},
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		switch int32(stack[0]) {
		case 1:
			return wantErr
		case 2:
			var m map[int]int
			m[0] = 1 // runtime panic
		case 3:
			panic("string panic")
		}
		stack[0] = uint64(int32(stack[0]) * 2)
		return nil
	})
	hComp, err := hb.Build(ctx)
	if err != nil {
		t.Fatalf("host Build: %v", err)
	}
	defer hComp.Close(ctx)

	wasm, err := os.ReadFile("../internal/core/testdata/consumer.wasm")
	if err != nil {
		t.Fatal(err)
	}
	comp, err := e.LoadComponent(ctx, strings.NewReader(string(wasm)))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	// quadruple(x) calls provider(x) twice. A trap poisons the host
	// instance, so each case gets fresh instances.
	call := func(x int32) ([]core.Val, error) {
		t.Helper()
		hInst, err := hComp.Instantiate(ctx)
		if err != nil {
			t.Fatalf("host Instantiate: %v", err)
		}
		defer hInst.Close(ctx)
		inst, err := comp.Instantiate(ctx, wacogo.WithFuncImport("provider", hInst.Core().ExportedFunc("provider")))
		if err != nil {
			t.Fatalf("Instantiate: %v", err)
		}
		defer inst.Close(ctx)
		return inst.ExportedFunc("quadruple").Call(ctx, core.ValS32(x))
	}

	res, err := call(5)
	if err != nil {
		t.Fatalf("quadruple(5): %v", err)
	}
	if got := int32(res[0].(core.ValS32)); got != 20 {
		t.Fatalf("quadruple(5) = %d, want 20", got)
	}

	for _, tc := range []struct {
		x    int32
		want []string
	}{
		{1, []string{wantErr.Error()}},
		{2, []string{"assignment to entry in nil map", "Go runtime stack trace"}},
		{3, []string{"string panic"}},
	} {
		_, err := call(tc.x)
		if err == nil {
			t.Fatalf("quadruple(%d): expected error", tc.x)
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("quadruple(%d) err = %q, want substring %q", tc.x, err.Error(), w)
			}
		}
	}
}
