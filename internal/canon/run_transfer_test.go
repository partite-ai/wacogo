package canon

import (
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo/internal/wasm"
)

func TestRunTransferPlan_TrapInStepReleasesTask(t *testing.T) {
	// Verify that a mid-execution trap causes tc.Task.End() to run,
	// draining any registered releases (e.g. unlends from LendTo).
	rt := &testResourceType{name: "R"}
	callerTable := newStubResourceTable()
	calleeTable := newStubResourceTable()
	srcH := callerTable.IssueOwn(rt, 42)

	// Pre-lend the handle via LendTo (which increments numLends and registers
	// an unlend release on the task). Capture the borrow in the unused callee slot.
	tc := &transferContext{
		caller:    &transferSide{Instance: &testInstance{}, ResourceTable: callerTable},
		callee:    &transferSide{Instance: &testInstance{}, ResourceTable: calleeTable},
		registers: make([]uint64, 4),
	}
	if _, err := srcH.LendTo(calleeTable, &tc.Task); err != nil {
		t.Fatalf("LendTo: %v", err)
	}

	plan := &transferPlan{
		paramSteps: nil,
		resultSteps: []transferPlanStep{
			{transfer: func(_ context.Context, _ *transferContext, _, _ uint32, _ *allocSource) {
				panic(&Trap{msg: "synthetic"})
			}},
		},
		coreStack: []uint64{},
	}
	calleeFn := newStubCoreFunc(t, nil, nil)
	defer func() {
		_ = recover()
		// After the trap, task.End() should have fired the unlend release.
		// Verify by transferring into a fresh table — succeeds only when
		// numLends == 0.
		if _, err := srcH.TransferOwn(newStubResourceTable()); err != nil {
			t.Errorf("TransferOwn after trap+task.End should succeed (numLends==0): %v", err)
		}
	}()
	runTransferPlan(context.Background(), plan, tc, calleeFn, nil, 0, nil)
}

func TestRunTransferPlan_TrapsOnUndroppedBorrow(t *testing.T) {
	// Verify that a call that issues a borrow (incrementing NumBorrows on
	// the callee) but does not drop it causes a trap at call resolution.
	defining := &testInstance{name: "defining"}
	rt := &resTypeWithInst{name: "R", inst: defining}
	callerTable := newStubResourceTable()
	// Use a callee table with a different owner to avoid the same-component
	// shortcut.
	calleeOwner := &testInstance{name: "callee"}
	calleeTable := newStubResourceTableFor(calleeOwner)
	ownH := callerTable.IssueOwn(rt, 99).HandleID()

	tc := &transferContext{
		caller:    &transferSide{Instance: &testInstance{}, ResourceTable: callerTable},
		callee:    &transferSide{Instance: calleeOwner, ResourceTable: calleeTable},
		registers: []uint64{uint64(ownH)},
	}
	plan := &transferPlan{
		paramSteps: []transferPlanStep{
			// Simulate what VisitBorrow does via the interface: LookupBorrowable + LendTo.
			{transfer: func(_ context.Context, tc *transferContext, _, _ uint32, _ *allocSource) {
				h := uint32(tc.registers[0])
				srcH, err := tc.caller.ResourceTable.LookupBorrowable(rt, h)
				if err != nil {
					panic(&Trap{msg: err.Error()})
				}
				dstH, err := srcH.LendTo(tc.callee.ResourceTable, &tc.Task)
				if err != nil {
					panic(&Trap{msg: err.Error()})
				}
				tc.registers[0] = uint64(dstH.HandleID())
				if tc.Task.NumBorrows != 1 {
					t.Errorf("paramStep: NumBorrows after LendTo = %d, want 1", tc.Task.NumBorrows)
				}
			}},
		},
		coreStack: make([]uint64, 1),
	}
	// Stub matches the runner's actual call shape: nCallerFlatParams=1
	// causes runTransferPlan to push one i64 to the wasm-side Call.
	calleeFn := newStubCoreFunc(t, []byte{wasm.ValI64}, nil)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected trap on undropped borrow")
		}
		trap, ok := r.(*Trap)
		if !ok {
			t.Fatalf("expected *Trap, got %T: %v", r, r)
		}
		if !strings.Contains(trap.msg, "1 outstanding borrow") {
			t.Errorf("expected trap msg containing %q, got %q", "1 outstanding borrow", trap.msg)
		}
	}()
	runTransferPlan(context.Background(), plan, tc, calleeFn, nil, 1, nil)
}
