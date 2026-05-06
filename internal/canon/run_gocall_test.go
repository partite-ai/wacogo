package canon

import (
	"context"
	"strings"
	"testing"
)

func TestRunGocallPlan_ErrorsOnUndroppedBorrow(t *testing.T) {
	rt := &testResourceType{name: "R"}
	calleeTable := newStubResourceTable()
	gcc := &gocallContext{callee: &transferSide{Instance: &testInstance{}, ResourceTable: calleeTable}}
	plan := &gocallPlan{
		paramSteps: []gocallLowerStep{
			func(_ context.Context, gcc *gocallContext, _ Val, _ uint32) error {
				_ = calleeTable.IssueBorrow(rt, 0, &gcc.Task)
				if gcc.Task.NumBorrows != 1 {
					t.Errorf("paramStep: NumBorrows after issueBorrow = %d, want 1", gcc.Task.NumBorrows)
				}
				return nil
			},
		},
		resultSteps: nil,
	}
	calleeFn := newStubCoreFunc(t, nil, nil)
	// The param step ignores its input; a nil-backed *ValOwnHandle is fine.
	_, err := runGocallPlan(context.Background(), plan, gcc, []Val{&ValOwnHandle{}}, calleeFn, nil)
	if err == nil || !strings.Contains(err.Error(), "1 outstanding borrow") {
		t.Errorf("expected outstanding-borrow error, got %v", err)
	}
}
