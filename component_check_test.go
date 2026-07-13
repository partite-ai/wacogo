package wacogo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasmtools"
)

const checkAPIImport = "example:check/api"

func loadCheckComponent(t *testing.T, engine *wacogo.Engine, wat string) *wacogo.Component {
	t.Helper()
	ctx := context.Background()
	tool, err := wasmtools.Default(ctx)
	if err != nil {
		t.Fatalf("wasmtools.Default: %v", err)
	}
	bin, err := tool.Parse(ctx, []byte(wat))
	if err != nil {
		t.Fatalf("wasmtools.Parse: %v", err)
	}
	component, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	return component
}

func loadCheckConsumer(t *testing.T, engine *wacogo.Engine) *wacogo.Component {
	t.Helper()
	return loadCheckComponent(t, engine, `(component
  (type $api (instance
    (export "ping" (func (param "value" u32) (result u32)))
  ))
  (import "example:check/api" (instance (type $api)))
)`)
}

func newCheckPingHost(t *testing.T, engine *wacogo.Engine, param host.TypeExpr) *host.ComponentInstance {
	t.Helper()
	ctx := context.Background()
	builder := engine.NewHostBuilder("check-instantiation-host")
	builder.AddFunction("ping", &host.FuncType{
		Params:  []host.Param{{Name: "value", Type: param}},
		Results: []host.ResultDecl{{Type: param}},
	}, func(context.Context, *host.CallContext, *host.ComponentInstance, []uint64) error {
		return nil
	})
	component, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("host Build: %v", err)
	}
	t.Cleanup(func() { _ = component.Close(ctx) })
	instance, err := component.Instantiate(ctx)
	if err != nil {
		t.Fatalf("host Instantiate: %v", err)
	}
	t.Cleanup(func() { _ = instance.Close(ctx) })
	return instance
}

func requireSharedImportError(t *testing.T, component *wacogo.Component, want string, opts ...wacogo.InstantiateOption) {
	t.Helper()
	checkErr := component.CheckInstantiation(opts...)
	if checkErr == nil || !strings.Contains(checkErr.Error(), want) {
		t.Fatalf("CheckInstantiation error = %v, want substring %q", checkErr, want)
	}

	_, instantiateErr := component.Instantiate(context.Background(), opts...)
	if instantiateErr == nil || !strings.Contains(instantiateErr.Error(), want) {
		t.Fatalf("Instantiate error = %v, want substring %q", instantiateErr, want)
	}
	if checkErr.Error() != instantiateErr.Error() {
		t.Fatalf("import validation drifted:\nCheckInstantiation: %v\nInstantiate: %v", checkErr, instantiateErr)
	}
}

func TestComponentCheckInstantiationAcceptsMatchingHostInterface(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	consumer := loadCheckConsumer(t, engine)
	matching := newCheckPingHost(t, engine, host.U32)
	opts := []wacogo.InstantiateOption{
		wacogo.WithInstanceImport(checkAPIImport, matching.Core()),
	}

	if err := consumer.CheckInstantiation(opts...); err != nil {
		t.Fatalf("CheckInstantiation: %v", err)
	}
	instance, err := consumer.Instantiate(ctx, opts...)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	t.Cleanup(func() { _ = instance.Close(ctx) })
}

func TestComponentCheckInstantiationRejectsMissingImport(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	requireSharedImportError(t, loadCheckConsumer(t, engine), "was not found")
}

func TestComponentCheckInstantiationRejectsSubtypeMismatch(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	consumer := loadCheckConsumer(t, engine)
	mismatched := newCheckPingHost(t, engine, host.U64)
	requireSharedImportError(t, consumer, "mismatch",
		wacogo.WithInstanceImport(checkAPIImport, mismatched.Core()))
}

func TestComponentCheckInstantiationDoesNotRunCoreStart(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	component := loadCheckComponent(t, engine, `(component
  (core module $trap
    (func $start unreachable)
    (start $start)
  )
  (core instance (instantiate $trap))
)`)

	if err := component.CheckInstantiation(); err != nil {
		t.Fatalf("CheckInstantiation ran the trapping start function: %v", err)
	}
	if _, err := component.Instantiate(ctx); err == nil {
		t.Fatal("Instantiate unexpectedly succeeded; fixture start function did not trap")
	}
}

func TestComponentCheckInstantiationRejectsWrongRuntimeKind(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	consumer := loadCheckConsumer(t, engine)
	hostInstance := newCheckPingHost(t, engine, host.U32)
	requireSharedImportError(t, consumer, "expected instance found function",
		wacogo.WithFuncImport(checkAPIImport, hostInstance.Core().ExportedFunc("ping")))
}

func TestComponentCheckInstantiationRejectsTypedNilProvider(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	var provider *wacogo.ComponentInstance
	requireSharedImportError(t, loadCheckConsumer(t, engine), "was not found",
		wacogo.WithInstanceImport(checkAPIImport, provider))
}

func TestComponentCheckInstantiationDoesNotCheckEngineLiveness(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	component := loadCheckComponent(t, engine, `(component)`)
	if err := engine.Close(ctx); err != nil {
		t.Fatalf("Engine.Close: %v", err)
	}

	if err := component.CheckInstantiation(); err != nil {
		t.Fatalf("metadata-only CheckInstantiation after Engine.Close: %v", err)
	}
	if _, err := component.Instantiate(ctx); err == nil || !strings.Contains(err.Error(), "engine is closed") {
		t.Fatalf("Instantiate after Engine.Close error = %v, want engine is closed", err)
	}
}
