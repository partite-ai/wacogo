package wacogo_test

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"unsafe"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/allocation"
)

type allocationObserver struct {
	calls  int
	nested [][]uint8
	absent []allocation.OptionWide
}

func (o *allocationObserver) Nested(_ context.Context, xs [][]uint8) error {
	o.calls++
	o.nested = xs
	return nil
}

func (o *allocationObserver) Absent(_ context.Context, xs []allocation.OptionWide) error {
	o.calls++
	o.absent = xs
	return nil
}

func TestGeneratedHostArgumentsRespectInlineStorage(t *testing.T) {
	for _, tc := range []struct {
		name, wit string
		storage   uint64
	}{
		{"nested", "(list (list u8))", 2 * uint64(unsafe.Sizeof([]uint8{}))},
		{"absent", "(list (option $wide))", 2 * uint64(unsafe.Sizeof(allocation.OptionWide{}))},
	} {
		wat := fmt.Sprintf(`(component
  (import "h" (instance $h
  (type $wide (record
    (field "a" u64) (field "b" u64) (field "c" u64) (field "d" u64)
    (field "e" u64) (field "f" u64) (field "g" u64) (field "h" u64)))
  (export "wide" (type (eq $wide)))
  (export "%s" (func (param "xs" %s)))))
  (alias export $h "%s" (func $consume))
  (core module $mem (memory (export "memory") 1))
  (core instance $memory (instantiate $mem))
  (core func $lowered (canon lower (func $consume) (memory (core memory $memory "memory"))))
  (core module $guest
    (import "host" "consume" (func $consume (param i32 i32)))
    (func (export "run") i32.const 128 i32.const 2 call $consume))
  (core instance $guest (instantiate $guest
    (with "host" (instance (export "consume" (func $lowered))))))
  (func (export "run") (canon lift (core func $guest "run"))))`, tc.name, tc.wit, tc.name)
		cmd := exec.Command("wasm-tools", "parse", "-")
		cmd.Stdin = strings.NewReader(wat)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		binary, err := cmd.Output()
		if err != nil {
			t.Fatalf("parse: %v: %s", err, stderr.String())
		}
		for _, path := range []string{"direct", "wasm"} {
			for _, budget := range []uint64{31, 32, tc.storage - 1, 4096, 0} {
				t.Run(fmt.Sprintf("%s/%s/%d", tc.name, path, budget), func(t *testing.T) {
					ctx := t.Context()
					engine := wacogo.NewEngine(ctx, wacogo.WithLiftLimits(wacogo.LiftLimits{MaxBytes: budget, MaxValues: 3}))
					defer engine.Close(ctx)
					factory, err := allocation.NewFactory(ctx, engine)
					if err != nil {
						t.Fatal(err)
					}
					defer factory.Close(ctx)
					observer := &allocationObserver{}
					instance, err := factory.NewInstance(ctx, observer, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer instance.Close(ctx)
					consume := instance.Core().ExportedFunc(tc.name)
					if path == "direct" {
						err = consume.CallRaw(ctx, nil, func(_, callee *host.CallContext, stack []uint64) {
							// Two zeroed list headers or absent 72-byte options.
							if !callee.Memory().Write(128, make([]byte, 144)) {
								t.Fatal("initialize arguments")
							}
							stack[0], stack[1] = 128, 2
						}, func(_, _ *host.CallContext, _ []uint64) {})
					} else {
						component, loadErr := engine.LoadComponent(ctx, bytes.NewReader(binary))
						if loadErr != nil {
							t.Fatal(loadErr)
						}
						guest, instErr := component.Instantiate(ctx, wacogo.WithInstanceImport("h", instance.Core()))
						if instErr != nil {
							t.Fatal(instErr)
						}
						defer guest.Close(ctx)
						_, err = guest.ExportedFunc("run").Call(ctx)
					}
					if budget != 0 && budget < tc.storage {
						if err == nil || !strings.Contains(err.Error(), wacogo.ErrLiftLimit.Error()) || observer.calls != 0 {
							t.Fatalf("generated host allocation exceeded budget: budget=%d backing=%d calls=%d err=%v", budget, tc.storage, observer.calls, err)
						}
					} else {
						if err != nil || observer.calls != 1 || len(observer.nested)+len(observer.absent) != 2 {
							t.Fatalf("valid arguments rejected: calls=%d err=%v", observer.calls, err)
						}
						for _, xs := range observer.nested {
							if len(xs) != 0 {
								t.Fatal("expected empty inner lists")
							}
						}
						for _, option := range observer.absent {
							if option.IsSome {
								t.Fatal("expected absent options")
							}
						}
					}
				})
			}
		}
	}
}
