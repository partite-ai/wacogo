package host_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// aliasedListFill writes a list<list<u8>> header block at 0x100: 64
// entries that all alias the same 1 KiB buffer at 0x1000. It occupies
// about 1.5 KiB of guest memory but lifts to 64*8 + 64*1024 = 66048
// canonical bytes.
const aliasedListFill = `
    (func $fill (local $i i32)
      (block $done (loop $l
        (br_if $done (i32.ge_u (local.get $i) (i32.const 64)))
        (i32.store (i32.add (i32.const 0x100) (i32.shl (local.get $i) (i32.const 3))) (i32.const 0x1000))
        (i32.store offset=4 (i32.add (i32.const 0x100) (i32.shl (local.get $i) (i32.const 3))) (i32.const 1024))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $l))))`

const aliasedListLiftedBytes = 64*8 + 64*1024

const bumpRealloc = `
    (global $bump (mut i32) (i32.const 0x8000))
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      (local $p i32)
      (local.set $p (global.get $bump))
      (global.set $bump (i32.add (global.get $bump) (local.get 3)))
      (local.get $p))`

// aliasedCalleeWAT exports get, returning the aliased list, and take,
// accepting one.
var aliasedCalleeWAT = fmt.Sprintf(`(component
  (core module $m
    (memory (export "mem") 4)
    %s
    %s
    (func (export "get") (result i32)
      (call $fill)
      (i32.store (i32.const 8) (i32.const 0x100))
      (i32.store (i32.const 12) (i32.const 64))
      (i32.const 8))
    (func (export "post") (param i32))
    (func (export "take") (param i32 i32))
    (func (export "huge") (result i32)
      (i32.store (i32.const 8) (i32.const 0))
      (i32.store (i32.const 12) (i32.const 0x1fffffff))
      (i32.const 8))
  )
  (core instance $i (instantiate $m))
  (func (export "get") (result (list (list u8)))
    (canon lift (core func $i "get") (memory $i "mem") (realloc (func $i "realloc"))
      (post-return (func $i "post"))))
  (func (export "take") (param "l" (list (list u8)))
    (canon lift (core func $i "take") (memory $i "mem") (realloc (func $i "realloc"))))
  (func (export "huge") (result (list u64))
    (canon lift (core func $i "huge") (memory $i "mem")))
)`, bumpRealloc, aliasedListFill)

// aliasedCallerWAT calls the callee's get (receiving the aliased list) or
// take (sending one).
var aliasedCallerWAT = fmt.Sprintf(`(component
  (import "callee" (instance $c
    (export "get" (func (result (list (list u8)))))
    (export "take" (func (param "l" (list (list u8)))))))
  (core module $libc
    (memory (export "mem") 4)
    %s)
  (core instance $libc (instantiate $libc))
  (core func $get (canon lower (func $c "get") (memory $libc "mem") (realloc (func $libc "realloc"))))
  (core func $take (canon lower (func $c "take") (memory $libc "mem")))
  (core module $m
    (import "libc" "mem" (memory 1))
    (import "host" "get" (func $get (param i32)))
    (import "host" "take" (func $take (param i32 i32)))
    %s
    (func (export "run-get") (call $get (i32.const 0)))
    (func (export "run-take") (call $fill) (call $take (i32.const 0x100) (i32.const 64))))
  (core instance $i (instantiate $m
    (with "libc" (instance $libc))
    (with "host" (instance (export "get" (func $get)) (export "take" (func $take))))))
  (func (export "run-get") (canon lift (core func $i "run-get")))
  (func (export "run-take") (canon lift (core func $i "run-take")))
)`, bumpRealloc, aliasedListFill)

func newLimitedEngine(t *testing.T, limit uint64) *wacogo.Engine {
	t.Helper()
	ctx := context.Background()
	e := wacogo.NewEngine(ctx, wacogo.WithMaxLiftBytes(limit))
	t.Cleanup(func() { _ = e.Close(ctx) })
	return e
}

func assertLiftLimitExceeded(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, wacogo.ErrLiftLimitExceeded) {
		t.Fatalf("err = %v, want one wrapping ErrLiftLimitExceeded", err)
	}
}

func TestLiftLimit_GoCallResult(t *testing.T) {
	ctx := context.Background()
	e := newLimitedEngine(t, aliasedListLiftedBytes-1)
	callee := instantiateWAT(t, e, aliasedCalleeWAT)

	_, err := callee.ExportedFunc("get").Call(ctx)
	assertLiftLimitExceeded(t, err)
	if callee.Poisoned() == nil {
		t.Fatal("callee not poisoned after exceeding the lift limit in its results")
	}
}

func TestLiftLimit_TransferResult(t *testing.T) {
	ctx := context.Background()
	e := newLimitedEngine(t, aliasedListLiftedBytes-1)
	callee := instantiateWAT(t, e, aliasedCalleeWAT)
	caller := instantiateWAT(t, e, aliasedCallerWAT, wacogo.WithInstanceImport("callee", callee))

	_, err := caller.ExportedFunc("run-get").Call(ctx)
	assertLiftLimitExceeded(t, err)
	if callee.Poisoned() == nil {
		t.Fatal("callee not poisoned after exceeding the lift limit in its results")
	}
}

func TestLiftLimit_TransferParams(t *testing.T) {
	ctx := context.Background()
	e := newLimitedEngine(t, aliasedListLiftedBytes-1)
	callee := instantiateWAT(t, e, aliasedCalleeWAT)
	caller := instantiateWAT(t, e, aliasedCallerWAT, wacogo.WithInstanceImport("callee", callee))

	_, err := caller.ExportedFunc("run-take").Call(ctx)
	assertLiftLimitExceeded(t, err)
	// The oversized value came from the caller; the callee never ran.
	if err := callee.Poisoned(); err != nil {
		t.Fatalf("callee poisoned by a caller's oversized arguments: %v", err)
	}
}

func TestLiftLimit_AtLimitSucceeds(t *testing.T) {
	ctx := context.Background()
	e := newLimitedEngine(t, aliasedListLiftedBytes)
	callee := instantiateWAT(t, e, aliasedCalleeWAT)
	caller := instantiateWAT(t, e, aliasedCallerWAT, wacogo.WithInstanceImport("callee", callee))

	res, err := callee.ExportedFunc("get").Call(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if n := res[0].(*wacogo.ValList).Len(); n != 64 {
		t.Fatalf("get returned %d elements, want 64", n)
	}
	if _, err := caller.ExportedFunc("run-get").Call(ctx); err != nil {
		t.Fatalf("run-get: %v", err)
	}
	if _, err := caller.ExportedFunc("run-take").Call(ctx); err != nil {
		t.Fatalf("run-take: %v", err)
	}
}

// TestGoLift_OversizedListHeaderRejected checks that a list header whose
// length runs past the end of memory is rejected before anything is
// allocated for it, even with no lift limit set.
func TestGoLift_OversizedListHeaderRejected(t *testing.T) {
	ctx := context.Background()
	e := newLimitedEngine(t, 0)
	callee := instantiateWAT(t, e, aliasedCalleeWAT)

	if _, err := callee.ExportedFunc("huge").Call(ctx); err == nil {
		t.Fatal("huge: expected error, got nil")
	}
}

func TestCallContextChargeLift(t *testing.T) {
	ctx := context.Background()
	const limit = 100
	e := newLimitedEngine(t, limit)

	hc, err := e.NewHostBuilder("caller").Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = hc.Close(ctx) })
	callerInst, err := hc.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	t.Cleanup(func() { _ = callerInst.Close(ctx) })

	callee := instantiateWAT(t, e, aliasedCalleeWAT)
	err = callee.ExportedFunc("get").CallRaw(ctx, callerInst.Core(),
		func(_, _ *host.CallContext, _ []uint64) {},
		func(_, callee *core.CallContext, _ []uint64) error {
			if err := callee.ChargeLift(limit); err != nil {
				return fmt.Errorf("charge up to limit: %w", err)
			}
			return callee.ChargeLift(1)
		},
	)
	assertLiftLimitExceeded(t, err)
	if callee.Poisoned() == nil {
		t.Fatal("callee not poisoned after its results exceeded the lift limit")
	}
}

// TestLiftLimit_TransferParamsToHost covers the serial (Go allocator)
// transfer path: a guest passing an oversized value to a host function.
func TestLiftLimit_TransferParamsToHost(t *testing.T) {
	for _, tc := range []struct {
		name     string
		limit    uint64
		wantCall bool
	}{
		{"over", aliasedListLiftedBytes - 1, false},
		{"at", aliasedListLiftedBytes, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			e := newLimitedEngine(t, tc.limit)

			listOfBytes := host.List{Elem: host.List{Elem: host.U8}}
			called := false
			b := e.NewHostBuilder("callee")
			b.AddFunction("get", &host.FuncType{
				Results: []host.ResultDecl{{"", listOfBytes}},
			}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
				return errors.New("unused")
			})
			b.AddFunction("take", &host.FuncType{
				Params: []host.Param{{"l", listOfBytes}},
			}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
				called = true
				return nil
			})
			hc, err := b.Build(ctx)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			t.Cleanup(func() { _ = hc.Close(ctx) })
			hostInst, err := hc.Instantiate(ctx)
			if err != nil {
				t.Fatalf("Instantiate: %v", err)
			}
			t.Cleanup(func() { _ = hostInst.Close(ctx) })

			caller := instantiateWAT(t, e, aliasedCallerWAT, wacogo.WithInstanceImport("callee", hostInst.Core()))
			_, err = caller.ExportedFunc("run-take").Call(ctx)
			if tc.wantCall {
				if err != nil {
					t.Fatalf("run-take: %v", err)
				}
			} else {
				assertLiftLimitExceeded(t, err)
			}
			if called != tc.wantCall {
				t.Fatalf("host take called = %v, want %v", called, tc.wantCall)
			}
		})
	}
}
