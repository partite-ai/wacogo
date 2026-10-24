package canon

import (
	"context"
	"errors"
	"fmt"
	"github.com/tetratelabs/wazero"
	"runtime"
	"strings"
	"testing"
)

type cleanupLiftInstance struct {
	testInstance
	table   *stubResourceTable
	entered bool
}

func (i *cleanupLiftInstance) Enter(context.Context) error {
	if i.entered {
		return errors.New("instance still entered")
	}
	i.entered = true
	return nil
}
func (i *cleanupLiftInstance) Exit(context.Context)         { i.entered = false }
func (i *cleanupLiftInstance) ResourceTable() ResourceTable { return i.table }

type cleanupLiftResource struct {
	testResourceTypeWithDtor
	instance Instance
}

func (r *cleanupLiftResource) DefiningInstance() Instance { return r.instance }

func TestLiftFailureReleasesAcquiredOwnAfterExit(t *testing.T) {
	for _, failure := range []string{"budget", "cancellation", "post-return", "none"} {
		t.Run(failure, func(t *testing.T) {
			mem := setupMem(t, 0)
			instance := &cleanupLiftInstance{table: newStubResourceTable()}
			dropped := 0
			resource := &cleanupLiftResource{
				instance:                 instance,
				testResourceTypeWithDtor: testResourceTypeWithDtor{dtor: func(context.Context, uint32) error { dropped++; return nil }},
			}
			handle := instance.table.IssueOwn(resource, 42)
			mem.WriteUint32Le(0, handle.HandleID())
			mem.WriteUint32Le(4, 64)
			mem.WriteUint32Le(8, 2)
			limits := LiftLimits{MaxValues: 16}
			if failure == "budget" {
				limits.MaxValues = 4
			}
			resultType := testTypeRecord{fields: []RecordField{
				{Name: "resource", Type: liftTestOwn{rt: resource}},
				{Name: "payload", Type: testTypeList{elem: testU8Type{}}},
			}}
			binding := NewCallBinding(nil, []Type{resultType}, Callee{
				CallSide: CallSide{Instance: instance, Memory: mem}, LiftLimits: limits,
				Direct: func(_ context.Context, stack []uint64) { stack[0] = 0 },
			})
			cleanupErr := errors.New("post-return failed")
			binding.postReturn = func(context.Context, []uint64) error {
				if !instance.entered {
					t.Fatal("post-return after Exit")
				}
				if failure == "post-return" {
					return cleanupErr
				}
				return nil
			}
			ctx := context.Background()
			if failure == "cancellation" {
				base, cancel := context.WithCancel(ctx)
				defer cancel()
				ctx = &cancelDuringLift{Context: base, cancel: cancel, checks: 6}
			}
			values, err := binding.Call(ctx, nil)
			if failure == "none" {
				if err != nil || dropped != 0 {
					t.Fatalf("success dropped result: drops=%d err=%v", dropped, err)
				}
				record := values[0].(*ValRecord)
				own := record.Fields()[0].Val.(*ValOwnHandle)
				if err := own.Drop(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || len(values) != 0 || dropped != 1 {
					t.Fatalf("failure leaked owned result: drops=%d values=%v err=%v", dropped, values, err)
				}
				switch failure {
				case "budget":
					if !errors.Is(err, ErrLiftLimit) {
						t.Fatal(err)
					}
				case "cancellation":
					if !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				case "post-return":
					if !errors.Is(err, cleanupErr) {
						t.Fatal(err)
					}
				}
			}
			if dropped != 1 || instance.entered {
				t.Fatalf("cleanup drops=%d entered=%t", dropped, instance.entered)
			}
			if _, err := instance.table.LookupOwn(resource, handle.HandleID()); err == nil {
				t.Fatal("source retained transferred resource")
			}
		})
	}
}

func TestHostSourcePreflightBeforeTranscoding(t *testing.T) {
	for _, enc := range []StringEncoding{EncUTF8, EncUTF16, EncLatin1UTF16} {
		t.Run(fmt.Sprint(enc), func(t *testing.T) {
			source, destination := setupMem(t, 0), setupMem(t, 0)
			coded := uint32(32768)
			data := []byte(strings.Repeat("a", int(coded)))
			if enc == EncUTF16 {
				coded /= 2
				for i := 1; i < len(data); i += 2 {
					data[i] = 0
				}
			}
			source.Write(0, data)
			reallocs, calls := 0, 0
			params := []Type{testTypeString{}}
			validator := NewLiftValidator(params, LiftLimits{MaxBytes: 1024})
			tc := &transferContext{
				caller: &transferSide{Instance: &testInstance{}, Memory: source, StringEncoding: enc},
				callee: &transferSide{Instance: &testInstance{}, Memory: destination, StringEncoding: EncUTF8, Arguments: validator,
					Realloc: func(context.Context, uint32, uint32, uint32, uint32) (uint32, error) { reallocs++; return 0, nil }},
				registers: []uint64{0, uint64(coded)},
			}
			plan := compileTransferPlan(params, nil, nil)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			var trapped error
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						if trap, ok := recovered.(error); ok {
							trapped = trap
						} else {
							panic(recovered)
						}
					}
				}()
				runTransferPlan(context.Background(), plan, tc, DirectFunc(func(context.Context, []uint64) { calls++ }), nil, 2, nil)
			}()
			runtime.ReadMemStats(&after)
			if trapped == nil || reallocs != 0 || calls != 0 {
				t.Fatalf("transfer began before rejection: trap=%v reallocs=%d calls=%d", trapped, reallocs, calls)
			}
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<10 {
				t.Fatalf("allocated %d bytes before rejecting source budget", allocated)
			}
		})
	}
}

func TestHostAdapterSourcePreflightBeforeAllocation(t *testing.T) {
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)
	source, destination := setupMem(t, 0), setupMem(t, 0)
	source.Write(0, []byte("a\x00b\x00c\x00d\x00"))
	params := []Type{testTypeString{}}
	reallocs, calls := 0, 0
	adapter, err := NewHost(runtime).BuildAdapter(ctx, "bounded-source",
		CallSide{Instance: &testInstance{}, Memory: source, StringEncoding: EncUTF16},
		Callee{
			CallSide: CallSide{Instance: &testInstance{}, Memory: destination, StringEncoding: EncUTF8,
				GoRealloc: func(context.Context, uint32, uint32, uint32, uint32) (uint32, error) { reallocs++; return 0, nil }},
			Arguments: NewLiftValidator(params, LiftLimits{MaxBytes: 16}),
			Direct:    func(context.Context, []uint64) { calls++ },
		}, params, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close(ctx)
	_, err = adapter.Module.ExportedFunction(adapter.Name).Call(ctx, 0, 4)
	if err == nil || !strings.Contains(err.Error(), ErrLiftLimit.Error()) || reallocs != 0 || calls != 0 {
		t.Fatalf("adapter dropped source preflight: calls=%d reallocs=%d err=%v", calls, reallocs, err)
	}
}

func TestLiftCleanupAllowsDestructorToReenterBinding(t *testing.T) {
	ctx := context.Background()
	mem := setupMem(t, 0)
	callee := &cleanupLiftInstance{table: newStubResourceTable()}
	owner := &testInstance{}
	var binding *CallBinding
	nested := false
	nestedErr := errors.New("nested core failure")
	drops := 0
	resource := &cleanupLiftResource{
		instance: owner,
		testResourceTypeWithDtor: testResourceTypeWithDtor{dtor: func(ctx context.Context, _ uint32) error {
			drops++
			if drops == 1 {
				nested = true
				_, err := binding.Call(ctx, nil)
				nested = false
				if !errors.Is(err, nestedErr) {
					t.Fatalf("nested call: %v", err)
				}
			}
			return nil
		}},
	}
	first := callee.table.IssueOwn(resource, 1)
	second := callee.table.IssueOwn(resource, 2)
	mem.WriteUint32Le(0, first.HandleID())
	mem.WriteUint32Le(4, second.HandleID())
	mem.WriteUint32Le(8, 64)
	mem.WriteUint32Le(12, 2)
	resultType := testTypeRecord{fields: []RecordField{
		{Name: "first", Type: liftTestOwn{rt: resource}},
		{Name: "second", Type: liftTestOwn{rt: resource}},
		{Name: "payload", Type: testTypeList{elem: testU8Type{}}},
	}}
	binding = NewCallBinding(nil, []Type{resultType}, Callee{
		CallSide: CallSide{Instance: callee, Memory: mem}, LiftLimits: LiftLimits{MaxValues: 5},
		Direct: func(_ context.Context, stack []uint64) {
			if nested {
				panic(nestedErr)
			}
			stack[0] = 0
		},
	})
	values, err := binding.Call(ctx, nil)
	if !errors.Is(err, ErrLiftLimit) || len(values) != 0 || drops != 2 || callee.entered {
		t.Fatalf("reentrant cleanup: values=%v drops=%d entered=%t err=%v", values, drops, callee.entered, err)
	}
}

func TestLiftCleanupContinuesAfterDestructorPanic(t *testing.T) {
	ctx := context.Background()
	mem := setupMem(t, 0)
	callee := &cleanupLiftInstance{table: newStubResourceTable()}
	owner := &testInstance{}
	dtorErr := errors.New("destructor panic")
	var order []uint32
	drops := 0
	resource := &cleanupLiftResource{
		instance: owner,
		testResourceTypeWithDtor: testResourceTypeWithDtor{dtor: func(_ context.Context, rep uint32) error {
			drops++
			order = append(order, rep)
			if rep == 2 {
				panic(dtorErr)
			}
			return nil
		}},
	}
	first := callee.table.IssueOwn(resource, 1)
	second := callee.table.IssueOwn(resource, 2)
	mem.WriteUint32Le(0, first.HandleID())
	mem.WriteUint32Le(4, second.HandleID())
	mem.WriteUint32Le(8, 64)
	mem.WriteUint32Le(12, 2)
	resultType := testTypeRecord{fields: []RecordField{
		{Name: "first", Type: liftTestOwn{rt: resource}},
		{Name: "second", Type: liftTestOwn{rt: resource}},
		{Name: "payload", Type: testTypeList{elem: testU8Type{}}},
	}}
	binding := NewCallBinding(nil, []Type{resultType}, Callee{
		CallSide: CallSide{Instance: callee, Memory: mem}, LiftLimits: LiftLimits{MaxValues: 5},
		Direct: func(_ context.Context, stack []uint64) {
			stack[0] = 0
		},
	})
	values, err := binding.Call(ctx, nil)
	if !errors.Is(err, dtorErr) || !errors.Is(err, ErrLiftLimit) || len(values) != 0 || drops != 2 || callee.entered {
		t.Fatalf("reentrant cleanup: values=%v drops=%d entered=%t err=%v", values, drops, callee.entered, err)
	}
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("drop order: %v", order)
	}
}
