package canon

import (
	"context"
	"errors"
	"testing"
	"unsafe"
)

func TestCallRawPreflightsResultAllocations(t *testing.T) {
	for _, scenario := range []string{"bytes", "values", "span", "success"} {
		t.Run(scenario, func(t *testing.T) {
			mem := setupMem(t, 0)
			mem.WriteUint32Le(0, 128)
			mem.WriteUint32Le(4, 2)
			limits := LiftLimits{MaxBytes: 32, MaxValues: 3}
			if scenario == "bytes" {
				limits.MaxBytes = 31
			}
			if scenario == "values" {
				limits.MaxValues = 2
			}
			if scenario == "span" {
				mem.WriteUint32Le(0, mem.Size()-1)
			}
			inst := &cleanupLiftInstance{table: newStubResourceTable()}
			binding := NewCallBinding(nil, []Type{testTypeList{elem: testU8Type{}}}, Callee{
				CallSide: CallSide{Instance: inst, Memory: mem}, LiftLimits: limits,
				Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
			})
			decoded, posted := 0, 0
			postErr := errors.New("post-return failure")
			binding.postReturn = func(context.Context, []uint64) error {
				posted++
				if !inst.entered {
					t.Error("post-return ran after Exit")
				}
				if scenario == "bytes" {
					return postErr
				}
				return nil
			}
			for attempt := 1; attempt <= 2; attempt++ {
				err := binding.CallRaw(context.Background(), func([]uint64) {}, func([]uint64) { decoded++ })
				if scenario == "success" {
					if err != nil || decoded != attempt {
						t.Fatalf("valid decode: count=%d err=%v", decoded, err)
					}
				} else if err == nil || decoded != 0 {
					t.Fatalf("unsafe decode: count=%d err=%v", decoded, err)
				}
				if (scenario == "bytes" || scenario == "values") && !errors.Is(err, ErrLiftLimit) {
					t.Errorf("missing limit error: %v", err)
				}
				if scenario == "bytes" && !errors.Is(err, postErr) {
					t.Errorf("missing post-return error: %v", err)
				}
				if posted != attempt || inst.entered {
					t.Fatalf("cleanup: post=%d entered=%t", posted, inst.entered)
				}
			}
		})
	}
}

func TestCallRawBudgetsInactiveInlinePayloads(t *testing.T) {
	type padded0 struct{ X uint32 }
	type padded1 struct {
		X padded0
		Z struct{}
	}
	type padded2 struct {
		X padded1
		Z struct{}
	}
	type padded3 struct {
		X padded2
		Z struct{}
	}
	type padded4 struct {
		X padded3
		Z struct{}
	}
	type option struct {
		IsSome bool
		Value  padded4
	}
	var inner Type = testTypeRecord{fields: []RecordField{{Name: "x", Type: testU32Type{}}}}
	for range 4 {
		inner = testTypeRecord{fields: []RecordField{
			{Name: "x", Type: inner}, {Name: "z", Type: testTypeRecord{}},
		}}
	}
	for _, tc := range []struct {
		name    string
		element Type
		actual  uint64
	}{
		{"absent padded option", testTypeOption{inner: inner}, uint64(unsafe.Sizeof(option{}))},
		{"empty nested lists", testTypeList{elem: testU8Type{}}, uint64(unsafe.Sizeof([]byte{}))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bound := max(valStorage, generatedInlineStorage(tc.element))
			if bound < tc.actual {
				t.Fatalf("bound=%d actual=%d", bound, tc.actual)
			}
			for _, budget := range []uint64{2*tc.actual - 1, 2 * bound} {
				mem := setupMem(t, 0)
				mem.WriteUint32Le(0, 128)
				mem.WriteUint32Le(4, 2)
				binding := NewCallBinding(nil, []Type{testTypeList{elem: tc.element}}, Callee{
					CallSide:   CallSide{Instance: &testInstance{}, Memory: mem},
					LiftLimits: LiftLimits{MaxBytes: budget, MaxValues: 3},
					Direct:     func(_ context.Context, stack []uint64) { stack[0] = 0 },
				})
				decoded := false
				err := binding.CallRaw(context.Background(), func([]uint64) {}, func([]uint64) { decoded = true })
				if budget < 2*tc.actual {
					if !errors.Is(err, ErrLiftLimit) || decoded {
						t.Fatalf("undercharged: decoded=%t err=%v", decoded, err)
					}
				} else if err != nil || !decoded {
					t.Fatalf("bound rejected: decoded=%t err=%v", decoded, err)
				}
			}
		})
	}
}

func TestCallRawRejectedResultKeepsOwnInCallee(t *testing.T) {
	mem := setupMem(t, 0)
	inst := &cleanupLiftInstance{table: newStubResourceTable()}
	resource := &testResourceType{}
	own := inst.table.IssueOwn(resource, 42)
	mem.WriteUint32Le(0, own.HandleID())
	mem.WriteUint32Le(4, 128)
	mem.WriteUint32Le(8, 64)
	result := testTypeRecord{fields: []RecordField{
		{Name: "own", Type: liftTestOwn{rt: resource}},
		{Name: "list", Type: testTypeList{elem: testU8Type{}}},
	}}
	binding := NewCallBinding(nil, []Type{result}, Callee{
		CallSide: CallSide{Instance: inst, Memory: mem}, LiftLimits: LiftLimits{MaxValues: 16},
		Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
	})
	err := binding.CallRaw(t.Context(), func([]uint64) {}, func([]uint64) { t.Fatal("decoded rejected result") })
	if !errors.Is(err, ErrLiftLimit) {
		t.Fatalf("result budget: %v", err)
	}
	if _, err := inst.table.LookupOwn(resource, own.HandleID()); err != nil {
		t.Fatalf("preflight consumed own: %v", err)
	}
}
