package fixturetests_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/calc"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/filesystem"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/streams"
)

// TestFactory_NewInstance_NilDepsWithImportsPanics verifies that passing
// nil for *Deps to a factory whose interface has imports panics with a
// "deps is nil" message.
func TestFactory_NewInstance_NilDepsWithImportsPanics(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Close(ctx)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on nil deps")
		}
		if !strings.Contains(fmt.Sprint(r), "deps is nil") {
			t.Errorf("panic message: got %v, want substring \"deps is nil\"", r)
		}
	}()
	_, _ = fac.NewInstance(ctx, &nilDepsTestImpl{}, nil)
}

// TestFactory_NewInstance_NilFieldPanicsListsField verifies that a Deps
// with a nil required field panics with the field name in the message.
func TestFactory_NewInstance_NilFieldPanicsListsField(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Close(ctx)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on Deps with nil field")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "Streams") {
			t.Errorf("panic message: got %v, want substring \"Streams\"", r)
		}
		if !strings.Contains(msg, "missing fields") {
			t.Errorf("panic message: got %v, want substring \"missing fields\"", r)
		}
	}()
	_, _ = fac.NewInstance(ctx, &nilDepsTestImpl{}, &filesystem.Deps{Streams: nil})
}

// TestFactory_NewInstance_NilDepsWithoutImportsAccepted verifies that a
// factory whose interface has no imports accepts nil for *Deps.
func TestFactory_NewInstance_NilDepsWithoutImportsAccepted(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := calc.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Close(ctx)

	inst, err := fac.NewInstance(ctx, calcStub{}, nil)
	if err != nil {
		t.Fatalf("NewInstance(nil deps): %v", err)
	}
	defer inst.Close(ctx)
}

// TestFactory_NewInstance_EmptyDepsLiteralAccepted verifies that an
// empty *Deps literal is also accepted by a no-imports factory.
func TestFactory_NewInstance_EmptyDepsLiteralAccepted(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := calc.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fac.Close(ctx)

	inst, err := fac.NewInstance(ctx, calcStub{}, &calc.Deps{})
	if err != nil {
		t.Fatalf("NewInstance(empty Deps): %v", err)
	}
	defer inst.Close(ctx)
}

// nilDepsTestImpl is a stub Filesystem impl whose methods will never be
// called — the tests panic before reaching any wasm dispatch.
type nilDepsTestImpl struct{}

func (*nilDepsTestImpl) Open(ctx context.Context) (*streams.HandleHandle, error) {
	return nil, fmt.Errorf("unreachable")
}

// calcStub is a no-op Calc impl.
type calcStub struct{}

func (calcStub) Add(ctx context.Context, a, b uint32) (uint32, error) { return a + b, nil }
