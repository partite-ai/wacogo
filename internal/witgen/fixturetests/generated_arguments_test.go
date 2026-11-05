package fixturetests_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"unsafe"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/allocation"
)

type allocationObserver struct {
	calls               int
	nested              [][]uint8
	strings             [][]string
	pairLeft, pairRight []string
	absent              []allocation.OptionWide
}

func (o *allocationObserver) Nested(_ context.Context, xs [][]uint8) error {
	o.calls++
	o.nested = xs
	return nil
}

func (o *allocationObserver) NestedStrings(_ context.Context, xs [][]string) error {
	o.calls++
	o.strings = xs
	return nil
}

func (o *allocationObserver) Pair(_ context.Context, left, right []string) error {
	o.calls++
	o.pairLeft, o.pairRight = left, right
	return nil
}

func (o *allocationObserver) Absent(_ context.Context, xs []allocation.OptionWide) error {
	o.calls++
	o.absent = xs
	return nil
}

func TestGeneratedHostNestedListsChargeGoStorage(t *testing.T) {
	const footprint = 2 * uint64(unsafe.Sizeof([]uint8{}))
	for _, path := range []string{"direct", "wasm"} {
		for _, budget := range []uint64{footprint - 1, footprint, 4096, 0} {
			t.Run(fmt.Sprintf("%s/budget-%d", path, budget), func(t *testing.T) {
				ctx := context.Background()
				e := wacogo.NewEngine(ctx, wacogo.WithMaxLiftBytes(budget))
				defer e.Close(ctx)
				factory, err := allocation.NewFactory(ctx, e)
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

				var callErr error
				if path == "direct" {
					callErr = instance.Core().ExportedFunc("nested").CallRaw(ctx, nil,
						func(_, callee *host.CallContext, stack []uint64) error {
							writeNestedEmpty(t, callee, stack)
							return nil
						},
						func(_, _ *host.CallContext, _ []uint64) error { return nil },
					)
				} else {
					guest := instantiateNestedGuest(t, ctx, e, instance)
					defer guest.Close(ctx)
					for range 3 {
						_, callErr = guest.ExportedFunc("run").Call(ctx)
						if callErr != nil {
							break
						}
					}
				}
				if budget != 0 && budget < footprint {
					if !errors.Is(callErr, wacogo.ErrLiftLimitExceeded) || observer.calls != 0 {
						t.Fatalf("under-budget host arguments: calls=%d err=%v, want zero calls and ErrLiftLimitExceeded", observer.calls, callErr)
					}
					return
				}
				wantCalls := 1
				if path == "wasm" {
					wantCalls = 3
				}
				if callErr != nil || observer.calls != wantCalls || len(observer.nested) != 2 {
					t.Fatalf("valid host arguments: calls=%d err=%v", observer.calls, callErr)
				}
				for _, xs := range observer.nested {
					if len(xs) != 0 {
						t.Fatal("expected empty inner lists")
					}
				}
			})
		}
	}
}

func writeNestedEmpty(t *testing.T, cc *host.CallContext, stack []uint64) {
	t.Helper()
	if !cc.Memory().Write(128, make([]byte, 16)) {
		t.Fatal("initialize nested list descriptors")
	}
	stack[0], stack[1] = 128, 2
}

func instantiateNestedGuest(t *testing.T, ctx context.Context, e *wacogo.Engine, instance *host.ComponentInstance) *wacogo.ComponentInstance {
	t.Helper()
	const guestWAT = `(component
  (import "h" (instance $h
    (export "nested" (func (param "xs" (list (list u8)))))))
  (alias export $h "nested" (func $consume))
  (core module $mem (memory (export "memory") 1))
  (core instance $memory (instantiate $mem))
  (alias core export $memory "memory" (core memory $mem))
  (core func $lowered (canon lower (func $consume) (memory $mem)))
  (core module $guest
    (import "host" "consume" (func $consume (param i32 i32)))
    (func (export "run") i32.const 128 i32.const 2 call $consume))
  (core instance $guest (instantiate $guest
    (with "host" (instance (export "consume" (func $lowered))))))
  (func (export "run") (canon lift (core func $guest "run"))))`
	component, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatal(err)
	}
	guest, err := component.Instantiate(ctx, wacogo.WithInstanceImport("h", instance.Core()))
	if err != nil {
		t.Fatal(err)
	}
	return guest
}

func TestGeneratedHostAbsentWideOptionsChargeGoStorage(t *testing.T) {
	const wireBytes = 2 * 72 // Canonical option<wide>: tag, padding, eight u64s.
	const goBytes = 2 * uint64(unsafe.Sizeof(allocation.OptionWide{}))
	for _, path := range []string{"direct", "wasm"} {
		footprint := goBytes
		if path == "wasm" {
			// Transfer and Go decoding each have a budget. Go's layout can be
			// smaller than the canonical layout on 32-bit platforms.
			footprint = max(footprint, wireBytes)
		}
		for _, budget := range []uint64{footprint - 1, footprint, 4096, 0} {
			t.Run(fmt.Sprintf("%s/budget-%d", path, budget), func(t *testing.T) {
				ctx := context.Background()
				e := wacogo.NewEngine(ctx, wacogo.WithMaxLiftBytes(budget))
				defer e.Close(ctx)
				factory, err := allocation.NewFactory(ctx, e)
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
				var callErr error
				if path == "direct" {
					callErr = instance.Core().ExportedFunc("absent").CallRaw(ctx, nil,
						func(_, callee *host.CallContext, stack []uint64) error {
							if !callee.Memory().Write(128, make([]byte, wireBytes)) {
								t.Fatal("initialize absent option elements")
							}
							stack[0], stack[1] = 128, 2
							return nil
						}, func(_, _ *host.CallContext, _ []uint64) error { return nil })
				} else {
					guest := instantiateAbsentGuest(t, ctx, e, instance)
					defer guest.Close(ctx)
					for range 3 {
						_, callErr = guest.ExportedFunc("run").Call(ctx)
						if callErr != nil {
							break
						}
					}
				}
				if budget != 0 && budget < footprint {
					if !errors.Is(callErr, wacogo.ErrLiftLimitExceeded) || observer.calls != 0 {
						t.Fatalf("under-budget absent options: calls=%d err=%v, want zero calls and ErrLiftLimitExceeded", observer.calls, callErr)
					}
					return
				}
				wantCalls := 1
				if path == "wasm" {
					wantCalls = 3
				}
				if callErr != nil || observer.calls != wantCalls || len(observer.absent) != 2 {
					t.Fatalf("valid absent options: calls=%d values=%d err=%v", observer.calls, len(observer.absent), callErr)
				}
				for _, option := range observer.absent {
					if option.IsSome {
						t.Fatal("expected absent option")
					}
				}
			})
		}
	}
}

func instantiateAbsentGuest(t *testing.T, ctx context.Context, e *wacogo.Engine, instance *host.ComponentInstance) *wacogo.ComponentInstance {
	t.Helper()
	const guestWAT = `(component
  (import "h" (instance $h
    (type $wide (record
      (field "a" u64) (field "b" u64) (field "c" u64) (field "d" u64)
      (field "e" u64) (field "f" u64) (field "g" u64) (field "h" u64)))
    (export "wide" (type (eq $wide)))
    (export "absent" (func (param "xs" (list (option $wide)))))))
  (alias export $h "absent" (func $consume))
  (core module $mem (memory (export "memory") 1))
  (core instance $memory (instantiate $mem))
  (alias core export $memory "memory" (core memory $mem))
  (core func $lowered (canon lower (func $consume) (memory $mem)))
  (core module $guest
    (import "host" "consume" (func $consume (param i32 i32)))
    (func (export "run") i32.const 128 i32.const 2 call $consume))
  (core instance $guest (instantiate $guest
    (with "host" (instance (export "consume" (func $lowered))))))
  (func (export "run") (canon lift (core func $guest "run"))))`
	component, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatal(err)
	}
	guest, err := component.Instantiate(ctx, wacogo.WithInstanceImport("h", instance.Core()))
	if err != nil {
		t.Fatal(err)
	}
	return guest
}

func TestGeneratedHostStringStorageSharesOneCallBudget(t *testing.T) {
	for _, tc := range []struct {
		name      string
		function  string
		footprint uint64
		setup     func(*testing.T, *host.CallContext, []uint64)
		check     func(*testing.T, *allocationObserver)
	}{
		{
			name:      "nested aliased strings",
			function:  "nested-strings",
			footprint: uint64(unsafe.Sizeof([]string{})) + 2*uint64(unsafe.Sizeof("")) + 2,
			setup: func(t *testing.T, cc *host.CallContext, stack []uint64) {
				mem := cc.Memory()
				writeU32(t, mem, 128, 256)
				writeU32(t, mem, 132, 2)
				for _, ptr := range []uint32{256, 264} {
					writeU32(t, mem, ptr, 512)
					writeU32(t, mem, ptr+4, 1)
				}
				if !mem.Write(512, []byte("x")) {
					t.Fatal("write aliased string bytes")
				}
				stack[0], stack[1] = 128, 1
			},
			check: func(t *testing.T, o *allocationObserver) {
				if len(o.strings) != 1 || len(o.strings[0]) != 2 || o.strings[0][0] != "x" || o.strings[0][1] != "x" {
					t.Fatalf("lifted aliased strings: %#v", o.strings)
				}
			},
		},
		{
			name:      "two arguments share budget",
			function:  "pair",
			footprint: 2*uint64(unsafe.Sizeof("")) + 2,
			setup: func(t *testing.T, cc *host.CallContext, stack []uint64) {
				mem := cc.Memory()
				for _, descriptor := range []uint32{128, 136} {
					writeU32(t, mem, descriptor, 512)
					writeU32(t, mem, descriptor+4, 1)
				}
				if !mem.Write(512, []byte("x")) {
					t.Fatal("write shared string bytes")
				}
				stack[0], stack[1], stack[2], stack[3] = 128, 1, 136, 1
			},
			check: func(t *testing.T, o *allocationObserver) {
				if len(o.pairLeft) != 1 || len(o.pairRight) != 1 || o.pairLeft[0] != "x" || o.pairRight[0] != "x" {
					t.Fatalf("lifted pair: left=%v right=%v", o.pairLeft, o.pairRight)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			for _, budget := range []uint64{tc.footprint - 1, tc.footprint, tc.footprint + 128, 0} {
				t.Run(fmt.Sprintf("budget-%d", budget), func(t *testing.T) {
					e := wacogo.NewEngine(ctx, wacogo.WithMaxLiftBytes(budget))
					defer e.Close(ctx)
					factory, err := allocation.NewFactory(ctx, e)
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
					fn := instance.Core().ExportedFunc(tc.function)
					call := func() error {
						return fn.CallRaw(ctx, nil, func(_, callee *host.CallContext, stack []uint64) error {
							tc.setup(t, callee, stack)
							return nil
						}, func(_, _ *host.CallContext, _ []uint64) error { return nil })
					}
					err = call()
					if budget != 0 && budget < tc.footprint {
						if !errors.Is(err, wacogo.ErrLiftLimitExceeded) || observer.calls != 0 {
							t.Fatalf("under-budget host arguments: calls=%d err=%v, want zero calls and ErrLiftLimitExceeded", observer.calls, err)
						}
						return
					}
					if err != nil || observer.calls != 1 {
						t.Fatalf("valid host arguments: calls=%d err=%v", observer.calls, err)
					}
					tc.check(t, observer)
					for range 2 {
						if err := call(); err != nil {
							t.Fatalf("repeated exact-budget call: %v", err)
						}
					}
					if observer.calls != 3 {
						t.Fatalf("calls after budget resets: %d, want 3", observer.calls)
					}
				})
			}
		})
	}
}

func writeU32(t *testing.T, mem interface{ WriteUint32Le(uint32, uint32) bool }, ptr, value uint32) {
	t.Helper()
	if !mem.WriteUint32Le(ptr, value) {
		t.Fatalf("write u32 at %d", ptr)
	}
}
