package host

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

// TestExportedFunc_CallRaw_TwoCallbacks verifies that the
// ExportedFunc CallRaw wrapper passes non-nil caller and callee
// CallContexts to both write and read callbacks, and that the two
// contexts bind to distinct *ComponentInstance values when caller !=
// calleeInst.
func TestExportedFunc_CallRaw_TwoCallbacks(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddFunction("add", &FuncType{
		Params:  []Param{{"a", U32}, {"b", U32}},
		Results: []ResultDecl{{"", U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		stack[0] = stack[0] + stack[1]
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	calleeInst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate callee: %v", err)
	}
	defer calleeInst.Close(ctx)

	callerInst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate caller: %v", err)
	}
	defer callerInst.Close(ctx)

	coreFn := calleeInst.Core().ExportedFunc("add")
	if coreFn == nil {
		t.Fatal("missing add export")
	}

	wrapped := WrapExportedFunc(coreFn)

	var (
		writeCallerInst, writeCalleeInst *core.ComponentInstance
		readCallerInst, readCalleeInst   *core.ComponentInstance
		readGot                          uint32
	)
	err = wrapped.CallRaw(ctx, callerInst,
		func(caller, callee *CallContext, stack []uint64) {
			if caller == nil || callee == nil {
				t.Fatal("write callback got nil context")
			}
			writeCallerInst = caller.Instance()
			writeCalleeInst = callee.Instance()
			stack[0] = 7
			stack[1] = 35
		},
		func(caller, callee *CallContext, stack []uint64) {
			if caller == nil || callee == nil {
				t.Fatal("read callback got nil context")
			}
			readCallerInst = caller.Instance()
			readCalleeInst = callee.Instance()
			readGot = uint32(stack[0])
		},
	)
	if err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if readGot != 42 {
		t.Fatalf("got %d, want 42", readGot)
	}
	if writeCallerInst != callerInst.Core() {
		t.Errorf("write caller.Instance() = %p, want %p", writeCallerInst, callerInst.Core())
	}
	if writeCalleeInst != calleeInst.Core() {
		t.Errorf("write callee.Instance() = %p, want %p", writeCalleeInst, calleeInst.Core())
	}
	if readCallerInst != callerInst.Core() {
		t.Errorf("read caller.Instance() = %p, want %p", readCallerInst, callerInst.Core())
	}
	if readCalleeInst != calleeInst.Core() {
		t.Errorf("read callee.Instance() = %p, want %p", readCalleeInst, calleeInst.Core())
	}
	if writeCallerInst == writeCalleeInst {
		t.Error("caller and callee Instance() coincide; want distinct when caller != calleeInst")
	}
}
