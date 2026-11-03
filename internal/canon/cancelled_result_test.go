package canon

import (
	"context"
	"errors"
	"testing"
)

func TestCallCancellationBeforeResultLift(t *testing.T) {
	for _, raw := range []bool{false, true} {
		name := "Call"
		if raw {
			name = "CallRaw"
		}
		t.Run(name, func(t *testing.T) {
			for _, cancelled := range []bool{false, true} {
				name := "active"
				if cancelled {
					name = "cancelled"
				}
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					mem := setupMem(t, 0)
					inst := &cancelledResultInstance{}
					calls, reads, posts := 0, 0, 0
					cb := NewCallBinding(nil, []Type{testTypeList{testU8Type{}}}, Callee{
						CallSide: CallSide{Instance: inst, Memory: mem, StringEncoding: EncUTF8},
						Direct: func(_ context.Context, stack []uint64) {
							calls++
							mem.WriteUint32Le(0, 128)
							mem.WriteUint32Le(4, 1000)
							stack[0] = 0
							if cancelled {
								cancel()
							}
						},
					}, 100000)
					cb.postReturn = func(context.Context, []uint64) error { posts++; return nil }
					var results []Val
					call := func(ctx context.Context) error {
						if raw {
							return cb.CallRaw(ctx, func([]uint64) error { return nil },
								func([]uint64, *LiftBudget) error { reads++; return nil })
						}
						var err error
						results, err = cb.Call(ctx, nil)
						return err
					}
					err := call(ctx)
					if cancelled {
						if !errors.Is(err, context.Canceled) || len(results) != 0 || reads != 0 || posts != 0 {
							t.Fatalf("cancelled call: err=%v results=%d reads=%d post-returns=%d", err, len(results), reads, posts)
						}
						if !errors.Is(inst.poisoned, context.Canceled) {
							t.Fatalf("cancelled result did not poison callee: %v", inst.poisoned)
						}
						if err := call(t.Context()); !errors.Is(err, context.Canceled) || calls != 1 {
							t.Fatalf("poisoned callee was reentered: err=%v calls=%d", err, calls)
						}
					} else if err != nil || posts != 1 || inst.poisoned != nil || (raw && reads != 1) || (!raw && len(results) != 1) {
						t.Fatalf("active call: err=%v results=%d reads=%d post-returns=%d poison=%v", err, len(results), reads, posts, inst.poisoned)
					}
					if inst.exits != 1 {
						t.Fatalf("callee exits=%d, want 1", inst.exits)
					}
				})
			}
		})
	}
}

func TestTransferCancellationBeforeResultLift(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	inst := &cancelledResultInstance{}
	transfers, posts := 0, 0
	tc := &transferContext{
		caller: &transferSide{Instance: &testInstance{}},
		callee: &transferSide{Instance: inst},
	}
	plan := &transferPlan{resultSteps: []transferPlanStep{
		{transfer: func(context.Context, *transferContext, uint32, uint32, *allocSource) { transfers++ }},
	}}
	fn := DirectFunc(func(context.Context, []uint64) { cancel() })
	postReturn := func(context.Context, []uint64) error { posts++; return nil }
	defer func() {
		cause, ok := recover().(error)
		if !ok || !errors.Is(cause, context.Canceled) || transfers != 0 || posts != 0 {
			t.Errorf("cancelled transfer: err=%v transfers=%d post-returns=%d", cause, transfers, posts)
		}
		if !errors.Is(inst.poisoned, context.Canceled) || inst.exits != 1 {
			t.Errorf("callee lifecycle: poison=%v exits=%d", inst.poisoned, inst.exits)
		}
	}()
	runTransferPlan(ctx, plan, tc, fn, postReturn, 0, nil)
}

type cancelledResultInstance struct {
	testInstance
	poisoned error
	exits    int
}

func (i *cancelledResultInstance) Poison(err error)            { i.poisoned = err }
func (i *cancelledResultInstance) Enter(context.Context) error { return i.poisoned }
func (i *cancelledResultInstance) Exit(context.Context)        { i.exits++ }
