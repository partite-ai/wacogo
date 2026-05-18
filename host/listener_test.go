package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

type event struct {
	phase string
	kind  CallKind
	name  string
	err   error
}

type recordingListener struct {
	events []event
}

func (r *recordingListener) BeforeCall(_ context.Context, _ *ComponentInstance, kind CallKind, name string, _ []uint64) {
	r.events = append(r.events, event{phase: "before", kind: kind, name: name})
}

func (r *recordingListener) AfterCall(_ context.Context, _ *ComponentInstance, kind CallKind, name string, _ []uint64, err error) {
	r.events = append(r.events, event{phase: "after", kind: kind, name: name, err: err})
}

func TestCallListener_FunctionSuccess(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddFunction("double", &FuncType{
		Params:  []Param{{"x", U32}},
		Results: []ResultDecl{{"", U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		stack[0] = stack[0] * 2
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	rec := &recordingListener{}
	inst, err := comp.Instantiate(ctx, WithCallListener(rec))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	if _, err := inst.Core().ExportedFunc("double").Call(ctx, core.ValU32(7)); err != nil {
		t.Fatalf("Call: %v", err)
	}

	if len(rec.events) != 2 {
		t.Fatalf("events: got %d, want 2: %+v", len(rec.events), rec.events)
	}
	if rec.events[0] != (event{phase: "before", kind: CallKindFunction, name: "double"}) {
		t.Fatalf("before event: %+v", rec.events[0])
	}
	if rec.events[1] != (event{phase: "after", kind: CallKindFunction, name: "double"}) {
		t.Fatalf("after event: %+v", rec.events[1])
	}
}

func TestCallListener_FunctionErrorReturn(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	sentinel := errors.New("boom")
	b.AddFunction("fail", &FuncType{}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error {
		return sentinel
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	rec := &recordingListener{}
	inst, err := comp.Instantiate(ctx, WithCallListener(rec))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	if _, err := inst.Core().ExportedFunc("fail").Call(ctx); err == nil {
		t.Fatal("Call: want error, got nil")
	}

	if len(rec.events) != 2 {
		t.Fatalf("events: got %d, want 2: %+v", len(rec.events), rec.events)
	}
	after := rec.events[1]
	if after.phase != "after" || !errors.Is(after.err, sentinel) {
		t.Fatalf("after event: %+v, want phase=after err=%v", after, sentinel)
	}
}

func TestCallListener_FunctionPanic(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddFunction("boom", &FuncType{}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error {
		panic("kaboom")
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	rec := &recordingListener{}
	inst, err := comp.Instantiate(ctx, WithCallListener(rec))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	if _, err := inst.Core().ExportedFunc("boom").Call(ctx); err == nil {
		t.Fatal("Call: want error after panic, got nil")
	}

	if len(rec.events) != 2 {
		t.Fatalf("events: got %d, want 2: %+v", len(rec.events), rec.events)
	}
	after := rec.events[1]
	if after.phase != "after" || after.err == nil || !strings.Contains(after.err.Error(), "kaboom") {
		t.Fatalf("after event after panic: %+v, want non-nil err containing %q", after, "kaboom")
	}
}

func TestInstrumentCall_DestructorKindObservedEvenWhenErr(t *testing.T) {
	// Direct unit test on instrumentCall: confirms destructor path
	// invokes the listener and that returned errors are observed but
	// not converted into panics (panicOnErr=false).
	rec := &recordingListener{}
	h := &ComponentInstance{callListener: rec}
	sentinel := errors.New("dtor failed")

	instrumentCall(context.Background(), h, CallKindDestructor, "my-resource",
		[]uint64{42}, false, func() error { return sentinel })

	if len(rec.events) != 2 {
		t.Fatalf("events: got %d, want 2: %+v", len(rec.events), rec.events)
	}
	if got := rec.events[0]; got.kind != CallKindDestructor || got.name != "my-resource" || got.phase != "before" {
		t.Fatalf("before: %+v", got)
	}
	if got := rec.events[1]; got.kind != CallKindDestructor || got.phase != "after" || !errors.Is(got.err, sentinel) {
		t.Fatalf("after: %+v", got)
	}
}

func TestInstrumentCall_PanicRethrownVerbatim(t *testing.T) {
	rec := &recordingListener{}
	h := &ComponentInstance{callListener: rec}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected re-thrown panic")
		}
		if got := fmt.Sprintf("%v", r); got != "boom" {
			t.Fatalf("panic value: got %q, want %q", got, "boom")
		}
		if len(rec.events) != 2 {
			t.Fatalf("events: got %d, want 2", len(rec.events))
		}
		if after := rec.events[1]; after.err == nil || !strings.Contains(after.err.Error(), "boom") {
			t.Fatalf("after.err: %v", after.err)
		}
	}()

	instrumentCall(context.Background(), h, CallKindFunction, "f",
		nil, true, func() error { panic("boom") })
}

func TestInstrumentCall_NilListenerFastPath(t *testing.T) {
	// Nil listener: no defer/recover. Returned err converts to a panic
	// when panicOnErr=true (matches the historical host-func behavior).
	h := &ComponentInstance{}
	sentinel := errors.New("err")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on returned error with panicOnErr=true")
		}
		if !errors.Is(r.(error), sentinel) {
			t.Fatalf("panic value: %v, want %v", r, sentinel)
		}
	}()

	instrumentCall(context.Background(), h, CallKindFunction, "f", nil, true,
		func() error { return sentinel })
}

func TestInstrumentCall_NilListenerDestructorSwallowsErr(t *testing.T) {
	h := &ComponentInstance{}
	sentinel := errors.New("err")
	// panicOnErr=false: should return without panicking
	instrumentCall(context.Background(), h, CallKindDestructor, "r", nil, false,
		func() error { return sentinel })
}
