package host

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

func TestHostArgumentsBudgetBeforeCallback(t *testing.T) {
	for _, transport := range []string{"direct", "wasm"} {
		for _, size := range []uint64{32, 33} {
			t.Run(transport+"/"+strings.Repeat("x", int(size)), func(t *testing.T) {
				ctx := context.Background()
				e := core.NewEngine(ctx, core.WithLiftLimits(core.LiftLimits{MaxBytes: 32, MaxValues: 1}))
				defer e.Close(ctx)
				b := NewBuilder(e, "bounded-host")
				called := 0
				b.AddFunction("log", &FuncType{Params: []Param{{Name: "message", Type: String}}}, func(_ context.Context, cc *core.CallContext, _ *ComponentInstance, stack []uint64) error {
					called++
					bytes, ok := cc.Memory().Read(uint32(stack[0]), uint32(stack[1]))
					if !ok {
						t.Fatal("callback got invalid memory")
					}
					if len(string(bytes)) != int(size) {
						t.Fatal("callback decoded wrong string")
					}
					return nil
				})
				comp, err := b.Build(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer comp.Close(ctx)
				inst, err := comp.Instantiate(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer inst.Close(ctx)
				inst.hostCallCC.Memory().Write(0, []byte(strings.Repeat("x", int(size))))
				if transport == "direct" {
					err = inst.Core().ExportedFunc("log").CallRaw(ctx, nil,
						func(_, _ *core.CallContext, stack []uint64) { stack[0] = 0; stack[1] = size },
						func(_, _ *core.CallContext, _ []uint64) {})
				} else {
					mod, buildErr := buildPerInstanceHostMod(ctx, core.WazeroRuntime(e), comp, inst)
					if buildErr != nil {
						t.Fatal(buildErr)
					}
					defer mod.Close(ctx)
					_, err = mod.ExportedFunction(comp.allFuncs[0].hostModExport).Call(ctx, 0, size)
				}
				if size == 32 {
					if err != nil || called != 1 {
						t.Fatalf("boundary: called=%d err=%v", called, err)
					}
				} else if called != 0 || err == nil || !strings.Contains(err.Error(), core.ErrLiftLimit.Error()) {
					t.Fatalf("oversize reached callback: called=%d err=%v", called, err)
				}
			})
		}
	}
}

func TestHostResultUsesEngineLiftLimits(t *testing.T) {
	ctx := context.Background()
	e := core.NewEngine(ctx, core.WithLiftLimits(core.LiftLimits{MaxValues: 2}))
	defer e.Close(ctx)
	b := NewBuilder(e, "bounded-result")
	b.AddFunction("list", &FuncType{Results: []ResultDecl{{Type: List{Elem: U8}}}}, func(_ context.Context, cc *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		cc.Memory().WriteUint32Le(0, 64)
		cc.Memory().WriteUint32Le(4, 2)
		stack[0] = 0
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close(ctx)
	if _, err := inst.Core().ExportedFunc("list").Call(ctx); !errors.Is(err, core.ErrLiftLimit) {
		t.Fatalf("engine result budget: %v", err)
	}
}
