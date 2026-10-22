package canon

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
)

func TestLiftRejectsOutOfBoundsListBeforeAllocation(t *testing.T) {
	mem := setupMem(t, 64)
	mem.WriteUint32Le(0, mem.Size()-4)
	mem.WriteUint32Le(4, 65536)
	binding := NewCallBinding(nil, []Type{testTypeList{elem: testU8Type{}}}, Callee{
		CallSide: CallSide{Instance: &testInstance{}, Memory: mem},
		Direct:   func(_ context.Context, stack []uint64) { stack[0] = 0 },
	})
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := binding.Call(context.Background(), nil)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("accepted an out-of-bounds list")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 128<<10 {
		t.Fatalf("allocated %d bytes before rejecting invalid guest memory", allocated)
	}
}

func TestLiftLimitsNestedAliasesAndReset(t *testing.T) {
	mem := setupMem(t, 0)
	mem.WriteUint32Le(0, 64)
	mem.WriteUint32Le(4, 4)
	for i := uint32(0); i < 4; i++ {
		mem.WriteUint32Le(64+i*8, 128)
		mem.WriteUint32Le(68+i*8, 16)
	}
	mem.Write(128, []byte("abcdefghijklmnop"))
	// Four aliases produce four independent Go string copies.
	exactBytes := valStorage + 4*valStorage + 4*16
	for _, tc := range []struct {
		name   string
		limits LiftLimits
		reject bool
	}{
		{"value boundary", LiftLimits{MaxValues: 5}, false},
		{"value exceeded", LiftLimits{MaxValues: 4}, true},
		{"byte boundary", LiftLimits{MaxBytes: exactBytes}, false},
		{"byte exceeded", LiftLimits{MaxBytes: exactBytes - 1}, true},
		{"both", LiftLimits{MaxValues: 5, MaxBytes: exactBytes}, false},
		{"unlimited", LiftLimits{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := NewCallBinding(nil, []Type{testTypeList{elem: testTypeString{}}}, Callee{
				CallSide: CallSide{Instance: &testInstance{}, Memory: mem}, LiftLimits: tc.limits,
				Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
			})
			for range 2 {
				values, err := binding.Call(context.Background(), nil)
				if tc.reject {
					if !errors.Is(err, ErrLiftLimit) {
						t.Fatalf("want lifting limit, got %v", err)
					}
					// A later valid call gets a fresh budget, including after failure.
					mem.WriteUint32Le(4, 1)
					if _, err := binding.Call(context.Background(), nil); err != nil {
						t.Fatalf("fresh call: %v", err)
					}
					mem.WriteUint32Le(4, 4)
				} else if err != nil || len(values) != 1 {
					t.Fatalf("want accepted result, got %v %v", values, err)
				}
			}
		})
	}
}

func TestLiftLimitsZeroStrideList(t *testing.T) {
	mem := setupMem(t, 0)
	mem.WriteUint32Le(0, 0)
	mem.WriteUint32Le(4, ^uint32(0))
	binding := NewCallBinding(nil, []Type{testTypeList{elem: testTypeRecord{}}}, Callee{
		CallSide: CallSide{Instance: &testInstance{}, Memory: mem}, LiftLimits: LiftLimits{MaxValues: 32, MaxBytes: 4096},
		Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
	})
	if _, err := binding.Call(context.Background(), nil); !errors.Is(err, ErrLiftLimit) {
		t.Fatalf("zero-stride list: %v", err)
	}
}

func TestLiftLimitsIndependentConcurrentCalls(t *testing.T) {
	mem := setupMem(t, 0)
	mem.WriteUint32Le(0, 64)
	mem.WriteUint32Le(4, 2)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		binding := NewCallBinding(nil, []Type{testTypeList{elem: testU8Type{}}}, Callee{
			CallSide: CallSide{Instance: &testInstance{}, Memory: mem}, LiftLimits: LiftLimits{MaxValues: 3, MaxBytes: 3 * valStorage},
			Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
		})
		wg.Go(func() {
			for range 100 {
				if _, err := binding.Call(context.Background(), nil); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestLiftLimitStillRunsPostReturn(t *testing.T) {
	mem := setupMem(t, 0)
	mem.WriteUint32Le(0, 64)
	mem.WriteUint32Le(4, 4)
	binding := NewCallBinding(nil, []Type{testTypeList{elem: testU8Type{}}}, Callee{
		CallSide: CallSide{Instance: &testInstance{}, Memory: mem}, LiftLimits: LiftLimits{MaxValues: 2},
		Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
	})
	cleanups := 0
	cleanupErr := errors.New("post-return failed")
	binding.postReturn = func(context.Context, []uint64) error { cleanups++; return cleanupErr }
	_, err := binding.Call(context.Background(), nil)
	if !errors.Is(err, ErrLiftLimit) || !errors.Is(err, cleanupErr) || cleanups != 1 {
		t.Fatalf("cleanup=%d err=%v", cleanups, err)
	}
	binding.postReturn = func(context.Context, []uint64) error { cleanups++; return nil }
	mem.WriteUint32Le(4, 1)
	if _, err := binding.Call(context.Background(), nil); err != nil || cleanups != 2 {
		t.Fatalf("cleanup=%d err=%v", cleanups, err)
	}
	binding.core = DirectFunc(func(context.Context, []uint64) { panic(errors.New("core trap")) })
	if _, err := binding.Call(context.Background(), nil); err == nil || cleanups != 2 {
		t.Fatalf("cleanup after trap=%d err=%v", cleanups, err)
	}
}

type cancelDuringLift struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelDuringLift) Err() error {
	c.checks++
	if c.checks == 10 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestLiftChecksCancellationInsideList(t *testing.T) {
	mem := setupMem(t, 0)
	mem.WriteUint32Le(0, 64)
	mem.WriteUint32Le(4, 100)
	binding := NewCallBinding(nil, []Type{testTypeList{elem: testU8Type{}}}, Callee{
		CallSide: CallSide{Instance: &testInstance{}, Memory: mem},
		Direct:   func(_ context.Context, stack []uint64) { stack[0] = 0 },
	})
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelDuringLift{Context: base, cancel: cancel}
	if _, err := binding.Call(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation, got %v", err)
	}
	if ctx.checks > 11 {
		t.Fatalf("continued lifting after cancellation: %d checks", ctx.checks)
	}
}

func TestLiftValidatorIndirectAndResourceArguments(t *testing.T) {
	mem := setupMem(t, 0)
	params := make([]Type, 17)
	for i := range params {
		params[i] = testTypeString{}
		mem.WriteUint32Le(uint32(i*8), 256)
		mem.WriteUint32Le(uint32(i*8+4), 4)
	}
	validator := NewLiftValidator(params, LiftLimits{MaxValues: 17, MaxBytes: 68})
	if err := validator.Validate(context.Background(), mem, EncUTF8, []uint64{0}); err != nil {
		t.Fatal(err)
	}
	validator = NewLiftValidator(params, LiftLimits{MaxBytes: 67})
	if err := validator.Validate(context.Background(), mem, EncUTF8, []uint64{0}); !errors.Is(err, ErrLiftLimit) {
		t.Fatalf("aliased indirect args: %v", err)
	}
	// Resource ownership must remain with the generated callback, not preflight.
	own := liftTestOwn{rt: &testResourceType{}}
	validator = NewLiftValidator([]Type{own}, LiftLimits{MaxValues: 1})
	if err := validator.Validate(context.Background(), mem, EncUTF8, []uint64{99}); err != nil {
		t.Fatalf("preflight transferred handle: %v", err)
	}
}

type liftTestOwn struct{ rt ResourceType }

func (t liftTestOwn) Accept(v TypeVisitor) { v.VisitOwn(t.rt) }

type liftTestBorrow struct{ rt ResourceType }

func (t liftTestBorrow) Accept(v TypeVisitor) { v.VisitBorrow(t.rt) }

func TestLiftValidatorBorrowFollowedByString(t *testing.T) {
	mem := setupMem(t, 0)
	validator := NewLiftValidator([]Type{liftTestBorrow{rt: &testResourceType{}}, testTypeString{}}, LiftLimits{MaxValues: 2, MaxBytes: 4})
	if err := validator.Validate(context.Background(), mem, EncUTF8, []uint64{99, 256, 4}); err != nil {
		t.Fatal(err)
	}
}
