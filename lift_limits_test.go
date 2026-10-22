package wacogo_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
)

func TestEngineLiftLimitsProtectImportedHostArguments(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx, wacogo.WithLiftLimits(wacogo.LiftLimits{MaxValues: 16, MaxBytes: 32}))
	defer engine.Close(ctx)
	builder := engine.NewHostBuilder("bounded-import")
	calls := 0
	builder.AddFunction("strlen", &host.FuncType{
		Params:  []host.Param{{Name: "s", Type: host.String}},
		Results: []host.ResultDecl{{Type: host.U32}},
	}, func(_ context.Context, cc *host.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		calls++
		data, ok := cc.Memory().Read(uint32(stack[0]), uint32(stack[1]))
		if !ok {
			t.Fatal("invalid callback memory")
		}
		stack[0] = uint64(len(string(data)))
		return nil
	})
	provider, err := builder.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close(ctx)
	binary, err := os.ReadFile("internal/core/testdata/string-consumer.wasm")
	if err != nil {
		t.Fatal(err)
	}
	component, err := engine.LoadComponent(ctx, bytes.NewReader(binary))
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{32, 33} {
		instance, err := provider.Instantiate(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer instance.Close(ctx)
		guest, err := component.Instantiate(ctx, wacogo.WithFuncImport("strlen", instance.Core().ExportedFunc("strlen")))
		if err != nil {
			t.Fatal(err)
		}
		defer guest.Close(ctx)
		_, err = guest.ExportedFunc("measure").Call(ctx, wacogo.ValString(strings.Repeat("x", size)))
		if size == 32 {
			if err != nil || calls != 1 {
				t.Fatalf("boundary: callback=%d err=%v", calls, err)
			}
		} else if err == nil || calls != 1 || !strings.Contains(err.Error(), wacogo.ErrLiftLimit.Error()) {
			t.Fatalf("oversize reached callback: callback=%d err=%v", calls, err)
		}
	}
}

func TestEngineLiftLimitsProtectGuestResults(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx, wacogo.WithLiftLimits(wacogo.LiftLimits{MaxBytes: 1}))
	defer engine.Close(ctx)
	binary, err := os.ReadFile("internal/core/testdata/simple-add.wasm")
	if err != nil {
		t.Fatal(err)
	}
	component, err := engine.LoadComponent(ctx, bytes.NewReader(binary))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := component.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close(ctx)
	if _, err := instance.ExportedFunc("add").Call(ctx, wacogo.ValS32(1), wacogo.ValS32(2)); !errors.Is(err, wacogo.ErrLiftLimit) {
		t.Fatalf("guest result: %v", err)
	}
}
