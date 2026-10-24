package host_test

import (
	"bytes"
	"context"
	"errors"
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
		func(_, _ *host.CallContext, _ []uint64) {},
		func(_, _ *core.CallContext, _ []uint64) error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("CallRaw err = %v, want %v", err, wantErr)
	}
	assertPoisonedWithoutPostReturn(t, err, callee)
}
