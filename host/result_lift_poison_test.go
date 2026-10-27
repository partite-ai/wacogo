package host_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// badResultCalleeWAT exports get: func() -> string whose result is
// invalid UTF-8. Its post-return traps, so any test that observes an
// "unreachable" error knows post-return ran when it should not have.
const badResultCalleeWAT = `(component
  (core module $m
    (memory (export "mem") 1)
    (data (i32.const 8) "\10\00\00\00\02\00\00\00\ff\ff")
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 1024)
    (func (export "get") (result i32) i32.const 8)
    (func (export "post") (param i32) unreachable)
  )
  (core instance $i (instantiate $m))
  (func (export "get") (result string)
    (canon lift (core func $i "get")
      (memory $i "mem")
      (realloc (func $i "realloc"))
      (post-return (func $i "post"))))
)`

// callerOfGetWAT imports get from a "callee" instance and calls it from
// its exported run function.
const callerOfGetWAT = `(component
  (import "callee" (instance $c (export "get" (func (result string)))))
  (core module $libc
    (memory (export "mem") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 1024))
  (core instance $libc (instantiate $libc))
  (core func $get (canon lower (func $c "get") (memory $libc "mem") (realloc (func $libc "realloc"))))
  (core module $m
    (import "host" "get" (func $get (param i32)))
    (func (export "run") (call $get (i32.const 0))))
  (core instance $i (instantiate $m (with "host" (instance (export "get" (func $get))))))
  (func (export "run") (canon lift (core func $i "run")))
)`

func instantiateWAT(t *testing.T, e *wacogo.Engine, wat string, opts ...wacogo.InstantiateOption) *wacogo.ComponentInstance {
	t.Helper()
	ctx := t.Context()
	comp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, wat)))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx, opts...)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	t.Cleanup(func() { _ = inst.Close(context.Background()) })
	return inst
}

func assertPoisonedWithoutPostReturn(t *testing.T, callErr error, inst *wacogo.ComponentInstance) {
	t.Helper()
	if callErr == nil {
		t.Fatal("call: expected error, got nil")
	}
	if strings.Contains(callErr.Error(), "unreachable") {
		t.Fatalf("call err = %q: post-return ran after a failed result lift", callErr)
	}
	if inst.Poisoned() == nil {
		t.Fatal("callee not poisoned after failed result lift")
	}
}

func TestResultLiftFailure_GoCallPoisonsCallee(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	callee := instantiateWAT(t, e, badResultCalleeWAT)
	_, err := callee.ExportedFunc("get").Call(ctx)
	assertPoisonedWithoutPostReturn(t, err, callee)
}

func TestResultLiftFailure_TransferPoisonsCallee(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	callee := instantiateWAT(t, e, badResultCalleeWAT)
	caller := instantiateWAT(t, e, callerOfGetWAT, wacogo.WithInstanceImport("callee", callee))
	_, err := caller.ExportedFunc("run").Call(ctx)
	assertPoisonedWithoutPostReturn(t, err, callee)
}

func TestResultLiftFailure_CallRawPoisonsCallee(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("caller")
	hc, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer hc.Close(ctx)
	callerInst, err := hc.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer callerInst.Close(ctx)

	callee := instantiateWAT(t, e, badResultCalleeWAT)
	wantErr := errors.New("read failed")
	err = callee.ExportedFunc("get").CallRaw(ctx, callerInst.Core(),
		func(_, _ *host.CallContext, _ []uint64) error { return nil },
		func(_, _ *core.CallContext, _ []uint64) error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("CallRaw err = %v, want %v", err, wantErr)
	}
	assertPoisonedWithoutPostReturn(t, err, callee)
}

// leavingPostReturnWAT exports get: func() -> u32 whose post-return calls
// resource.new, which may_leave forbids during post-return.
const leavingPostReturnWAT = `(component
  (type $r (resource (rep i32)))
  (core func $new (canon resource.new $r))
  (core module $m
    (import "r" "new" (func $new (param i32) (result i32)))
    (func (export "get") (result i32) i32.const 7)
    (func (export "post") (param i32) (drop (call $new (i32.const 1)))))
  (core instance $i (instantiate $m (with "r" (instance (export "new" (func $new))))))
  (func (export "get") (result u32)
    (canon lift (core func $i "get") (post-return (func $i "post"))))
)`

// TestPostReturn_MayNotLeave checks post-return runs under may_leave=false
// on both the Call and CallRaw paths.
func TestPostReturn_MayNotLeave(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	callee := instantiateWAT(t, e, leavingPostReturnWAT)
	if _, err := callee.ExportedFunc("get").Call(ctx); err == nil || !strings.Contains(err.Error(), "may_leave") {
		t.Fatalf("Call: err = %v, want a may_leave violation", err)
	}

	hc, err := e.NewHostBuilder("caller").Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer hc.Close(ctx)
	callerInst, err := hc.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer callerInst.Close(ctx)

	callee = instantiateWAT(t, e, leavingPostReturnWAT)
	err = callee.ExportedFunc("get").CallRaw(ctx, callerInst.Core(),
		func(_, _ *host.CallContext, _ []uint64) error { return nil },
		func(_, _ *core.CallContext, _ []uint64) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "may_leave") {
		t.Fatalf("CallRaw: err = %v, want a may_leave violation", err)
	}
}

// reallocCalleeWAT exports take: func(s: string) whose realloc is given
// by the %s body: it receives (orig-ptr, orig-size, align, new-size).
const reallocCalleeWAT = `(component
  (core module $m
    (memory (export "mem") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) %s)
    (func (export "take") (param i32 i32)))
  (core instance $i (instantiate $m))
  (func (export "take") (param "s" string)
    (canon lift (core func $i "take") (memory $i "mem") (realloc (func $i "realloc"))))
)`

// leavingReallocWAT is reallocCalleeWAT with a realloc that calls
// resource.new, which may_leave forbids while arguments are lowered.
const leavingReallocWAT = `(component
  (type $r (resource (rep i32)))
  (core func $new (canon resource.new $r))
  (core module $m
    (import "r" "new" (func $new (param i32) (result i32)))
    (memory (export "mem") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      (drop (call $new (i32.const 1)))
      i32.const 16)
    (func (export "take") (param i32 i32)))
  (core instance $i (instantiate $m (with "r" (instance (export "new" (func $new))))))
  (func (export "take") (param "s" string)
    (canon lift (core func $i "take") (memory $i "mem") (realloc (func $i "realloc"))))
)`

// TestCallRaw_ReallocMayNotLeave checks the callee's realloc runs under
// may_leave=false while a write callback lowers arguments.
func TestCallRaw_ReallocMayNotLeave(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	hc, err := e.NewHostBuilder("caller").Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer hc.Close(ctx)
	callerInst, err := hc.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer callerInst.Close(ctx)

	callee := instantiateWAT(t, e, leavingReallocWAT)
	err = callee.ExportedFunc("take").CallRaw(ctx, callerInst.Core(),
		func(_, callee *host.CallContext, _ []uint64) error {
			_, err := callee.Realloc(ctx, 0, 0, 2, 4)
			return err
		},
		func(_, _ *core.CallContext, _ []uint64) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "may_leave") {
		t.Fatalf("CallRaw: err = %v, want a may_leave violation", err)
	}
}

// TestCallRaw_ReallocFailurePoisonsCallee checks that a write callback's
// call to the callee's realloc fails, and poisons the callee, when the
// realloc traps or returns a pointer the canonical ABI rejects. Address 0
// is valid, so a null pointer is not rejected.
func TestCallRaw_ReallocFailurePoisonsCallee(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"traps", "unreachable"},
		{"out of bounds", "i32.const 0x10000"},
		{"misaligned", "i32.const 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			e := wacogo.NewEngine(ctx)
			defer e.Close(ctx)

			hc, err := e.NewHostBuilder("caller").Build(ctx)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			defer hc.Close(ctx)
			callerInst, err := hc.Instantiate(ctx)
			if err != nil {
				t.Fatalf("Instantiate: %v", err)
			}
			defer callerInst.Close(ctx)

			callee := instantiateWAT(t, e, fmt.Sprintf(reallocCalleeWAT, tc.body))
			err = callee.ExportedFunc("take").CallRaw(ctx, callerInst.Core(),
				func(_, callee *host.CallContext, _ []uint64) error {
					_, err := callee.Realloc(ctx, 0, 0, 2, 4)
					return err
				},
				func(_, _ *core.CallContext, _ []uint64) error { return nil },
			)
			if err == nil {
				t.Fatal("CallRaw: expected the realloc failure, got nil")
			}
			if callee.Poisoned() == nil {
				t.Fatal("callee not poisoned after its realloc failed")
			}
		})
	}
}
