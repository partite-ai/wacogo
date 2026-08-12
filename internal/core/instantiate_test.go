package core

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestInstantiate_SimpleAdd(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	addFunc := inst.ExportedFunc("add")
	if addFunc == nil {
		t.Fatal("ExportedFunc(\"add\") returned nil")
	}

	// Verify the function type.
	ft := addFunc.Type()
	if ft == nil {
		t.Fatal("add function has nil FuncType")
	}
	if len(ft.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(ft.Params))
	}
	if len(ft.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(ft.Results))
	}

	// Call add(3, 4) and expect 7.
	results, err := addFunc.Call(ctx, ValS32(3), ValS32(4))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got, ok := results[0].(ValS32)
	if !ok {
		t.Fatalf("expected ValS32 result, got %T", results[0])
	}
	if got != 7 {
		t.Fatalf("add(3, 4) = %d, want 7", got)
	}
}

func TestInstantiate_TwoComponentLinking(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	// Load and instantiate the provider component.
	pf, err := os.Open("testdata/provider.wasm")
	if err != nil {
		t.Fatalf("open provider: %v", err)
	}
	defer pf.Close()

	providerComp, err := engine.LoadComponent(ctx, pf)
	if err != nil {
		t.Fatalf("LoadComponent(provider): %v", err)
	}

	providerInst, err := providerComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate(provider): %v", err)
	}
	defer providerInst.Close(ctx)

	// Verify the provider works standalone.
	doubleFunc := providerInst.ExportedFunc("double")
	if doubleFunc == nil {
		t.Fatal("provider: ExportedFunc(\"double\") returned nil")
	}
	results, err := doubleFunc.Call(ctx, ValS32(5))
	if err != nil {
		t.Fatalf("provider double(5): %v", err)
	}
	if got := results[0].(ValS32); got != 10 {
		t.Fatalf("provider double(5) = %d, want 10", got)
	}

	// Load and instantiate the consumer component with provider linked in.
	cf, err := os.Open("testdata/consumer.wasm")
	if err != nil {
		t.Fatalf("open consumer: %v", err)
	}
	defer cf.Close()

	consumerComp, err := engine.LoadComponent(ctx, cf)
	if err != nil {
		t.Fatalf("LoadComponent(consumer): %v", err)
	}

	consumerInst, err := consumerComp.Instantiate(ctx, WithFuncImport("provider", providerInst.ExportedFunc("double")))
	if err != nil {
		t.Fatalf("Instantiate(consumer): %v", err)
	}
	defer consumerInst.Close(ctx)

	// Call quadruple(3) and expect 12.
	quadrupleFunc := consumerInst.ExportedFunc("quadruple")
	if quadrupleFunc == nil {
		t.Fatal("consumer: ExportedFunc(\"quadruple\") returned nil")
	}
	results, err = quadrupleFunc.Call(ctx, ValS32(3))
	if err != nil {
		t.Fatalf("consumer quadruple(3): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got, ok := results[0].(ValS32)
	if !ok {
		t.Fatalf("expected ValS32 result, got %T", results[0])
	}
	if got != 12 {
		t.Fatalf("quadruple(3) = %d, want 12", got)
	}

	// Also test with different values.
	results, err = quadrupleFunc.Call(ctx, ValS32(0))
	if err != nil {
		t.Fatalf("consumer quadruple(0): %v", err)
	}
	if results[0].(ValS32) != 0 {
		t.Fatalf("quadruple(0) = %d, want 0", results[0])
	}

	results, err = quadrupleFunc.Call(ctx, ValS32(7))
	if err != nil {
		t.Fatalf("consumer quadruple(7): %v", err)
	}
	if results[0].(ValS32) != 28 {
		t.Fatalf("quadruple(7) = %d, want 28", results[0])
	}
}

func TestFuncCall_String(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	f, err := os.Open("testdata/strlen.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	strlen := inst.ExportedFunc("strlen")
	if strlen == nil {
		t.Fatal("ExportedFunc(\"strlen\") returned nil")
	}

	// "hello" is 5 UTF-8 bytes.
	results, err := strlen.Call(ctx, ValString("hello"))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if results[0] != ValU32(5) {
		t.Fatalf("strlen('hello') = %v, want 5", results[0])
	}

	// Empty string.
	results, err = strlen.Call(ctx, ValString(""))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if results[0] != ValU32(0) {
		t.Fatalf("strlen('') = %v, want 0", results[0])
	}

	// Unicode string: "héllo" = 6 UTF-8 bytes.
	results, err = strlen.Call(ctx, ValString("héllo"))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if results[0] != ValU32(6) {
		t.Fatalf("strlen('héllo') = %v, want 6", results[0])
	}
}

func TestInstantiate_TwoComponentLinking_ResourceTable(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	pf, err := os.Open("testdata/provider.wasm")
	if err != nil {
		t.Fatalf("open provider: %v", err)
	}
	defer pf.Close()

	providerComp, err := engine.LoadComponent(ctx, pf)
	if err != nil {
		t.Fatalf("LoadComponent(provider): %v", err)
	}

	providerInst, err := providerComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate(provider): %v", err)
	}
	defer providerInst.Close(ctx)

	cf, err := os.Open("testdata/consumer.wasm")
	if err != nil {
		t.Fatalf("open consumer: %v", err)
	}
	defer cf.Close()

	consumerComp, err := engine.LoadComponent(ctx, cf)
	if err != nil {
		t.Fatalf("LoadComponent(consumer): %v", err)
	}

	consumerInst, err := consumerComp.Instantiate(ctx, WithFuncImport("provider", providerInst.ExportedFunc("double")))
	if err != nil {
		t.Fatalf("Instantiate(consumer): %v", err)
	}
	defer consumerInst.Close(ctx)

	// Verify linking still works correctly.
	quadrupleFunc := consumerInst.ExportedFunc("quadruple")
	if quadrupleFunc == nil {
		t.Fatal("consumer: ExportedFunc(\"quadruple\") returned nil")
	}
	results, err := quadrupleFunc.Call(ctx, ValS32(5))
	if err != nil {
		t.Fatalf("consumer quadruple(5): %v", err)
	}
	if got := results[0].(ValS32); got != 20 {
		t.Fatalf("quadruple(5) = %d, want 20", got)
	}
}

func TestInstantiate_ExportedFuncNotFound(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	if inst.ExportedFunc("nonexistent") != nil {
		t.Fatal("expected nil for nonexistent function")
	}
}

// TestInstantiate_TwoComponentLinking_String tests cross-component linking
// where a string value is passed across the boundary using a FACT adapter.
// The provider exports strlen(string) -> u32 and the consumer imports it,
// lowering with memory and realloc so the adapter can read strings from the
// consumer's memory and copy them to the provider's memory.
func TestInstantiate_TwoComponentLinking_String(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	// Load and instantiate the string provider.
	pf, err := os.Open("testdata/string-provider.wasm")
	if err != nil {
		t.Fatalf("open string-provider: %v", err)
	}
	defer pf.Close()

	providerComp, err := engine.LoadComponent(ctx, pf)
	if err != nil {
		t.Fatalf("LoadComponent(string-provider): %v", err)
	}

	providerInst, err := providerComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate(string-provider): %v", err)
	}
	defer providerInst.Close(ctx)

	// Verify the provider works standalone.
	strlenFunc := providerInst.ExportedFunc("strlen")
	if strlenFunc == nil {
		t.Fatal("provider: ExportedFunc(\"strlen\") returned nil")
	}
	results, err := strlenFunc.Call(ctx, ValString("hello"))
	if err != nil {
		t.Fatalf("provider strlen(\"hello\"): %v", err)
	}
	if got := results[0].(ValU32); got != 5 {
		t.Fatalf("provider strlen(\"hello\") = %d, want 5", got)
	}

	// Load and instantiate the string consumer with provider linked in.
	cf, err := os.Open("testdata/string-consumer.wasm")
	if err != nil {
		t.Fatalf("open string-consumer: %v", err)
	}
	defer cf.Close()

	consumerComp, err := engine.LoadComponent(ctx, cf)
	if err != nil {
		t.Fatalf("LoadComponent(string-consumer): %v", err)
	}

	consumerInst, err := consumerComp.Instantiate(ctx, WithFuncImport("strlen", providerInst.ExportedFunc("strlen")))
	if err != nil {
		t.Fatalf("Instantiate(string-consumer): %v", err)
	}
	defer consumerInst.Close(ctx)

	// Call measure("hello") and expect 5.
	measureFunc := consumerInst.ExportedFunc("measure")
	if measureFunc == nil {
		t.Fatal("consumer: ExportedFunc(\"measure\") returned nil")
	}

	results, err = measureFunc.Call(ctx, ValString("hello"))
	if err != nil {
		t.Fatalf("consumer measure(\"hello\"): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got, ok := results[0].(ValU32)
	if !ok {
		t.Fatalf("expected ValU32 result, got %T", results[0])
	}
	if got != 5 {
		t.Fatalf("measure(\"hello\") = %d, want 5", got)
	}

	// Test with empty string.
	results, err = measureFunc.Call(ctx, ValString(""))
	if err != nil {
		t.Fatalf("consumer measure(\"\"): %v", err)
	}
	if results[0].(ValU32) != 0 {
		t.Fatalf("measure(\"\") = %v, want 0", results[0])
	}

	// Test with Unicode string: "cafe\u0301" = 6 UTF-8 bytes.
	results, err = measureFunc.Call(ctx, ValString("h\u00e9llo"))
	if err != nil {
		t.Fatalf("consumer measure(\"h\\u00e9llo\"): %v", err)
	}
	if results[0].(ValU32) != 6 {
		t.Fatalf("measure(\"h\\u00e9llo\") = %v, want 6", results[0])
	}
}

func TestInstantiateMintsFreshInstanceType(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	comp, err := e.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	inst1, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate #1: %v", err)
	}
	inst2, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate #2: %v", err)
	}
	if inst1.wpInstance == nil || inst2.wpInstance == nil {
		t.Fatal("expected instance type handles to be populated")
	}
	if inst1.wpInstance == inst2.wpInstance {
		t.Fatal("want distinct InstanceType handles per Instantiate call")
	}
}

func TestInstantiateRejectsMismatchedResourceTypes(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	provWAT := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	consWAT := `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`
	provBin := buildComponentBytes(t, provWAT)
	consBin := buildComponentBytes(t, consWAT)

	prov, err := e.LoadComponent(ctx, bytes.NewReader(provBin))
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	cons, err := e.LoadComponent(ctx, bytes.NewReader(consBin))
	if err != nil {
		t.Fatalf("load consumer: %v", err)
	}
	i1, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatalf("inst prov #1: %v", err)
	}
	i2, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatalf("inst prov #2: %v", err)
	}

	_, err = cons.Instantiate(ctx,
		WithInstanceImport("I1", i1),
		WithInstanceImport("I2", i2),
	)
	if err == nil {
		t.Fatal("expected instantiation to fail, got nil")
	}
	// Error phrasing alignment is T9's job; for now, just ensure it
	// surfaces *some* type-level rejection (not, e.g., a runtime trap).
	if !strings.Contains(err.Error(), "resource") {
		t.Fatalf("expected resource-related error, got %q", err.Error())
	}
}

func TestInstantiateAcceptsSharedResource(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	provWAT := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	consWAT := `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`
	provBin := buildComponentBytes(t, provWAT)
	consBin := buildComponentBytes(t, consWAT)

	prov, err := e.LoadComponent(ctx, bytes.NewReader(provBin))
	if err != nil {
		t.Fatal(err)
	}
	cons, err := e.LoadComponent(ctx, bytes.NewReader(consBin))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cons.Instantiate(ctx,
		WithInstanceImport("I1", shared),
		WithInstanceImport("I2", shared),
	)
	if err != nil {
		t.Fatalf("expected success with shared instance, got %v", err)
	}
}

func TestInstantiateRejectsFuncArgWithWrongSignature(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	consBin := buildComponentBytes(t, `(component
  (type (func (param "x" u32) (result u32)))
  (import "f" (func (type 0)))
)`)
	provBin := buildComponentBytes(t, `(component
  (core module $m (func (export "f") (param i32 i32) (result i32) local.get 0))
  (core instance $i (instantiate $m))
  (func (export "f") (param "x" u32) (param "y" u32) (result u32) (canon lift (core func $i "f")))
)`)
	cons, err := e.LoadComponent(ctx, bytes.NewReader(consBin))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := e.LoadComponent(ctx, bytes.NewReader(provBin))
	if err != nil {
		t.Fatal(err)
	}
	provInst, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fn := provInst.ExportedFunc("f")
	if fn == nil {
		t.Fatal("provider did not export f")
	}
	_, err = cons.Instantiate(ctx, WithFuncImport("f", fn))
	if err == nil {
		t.Fatal("expected signature-mismatch error from the pre-check, got nil")
	}
}

func TestInstantiateAcceptsMatchingModuleArg(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	consBin := buildComponentBytes(t, `(component
  (core type $mt (module (export "f" (func))))
  (import "m" (core module (type $mt)))
)`)
	provBin := buildComponentBytes(t, `(component
  (core module $m (func (export "f")))
  (export "m" (core module $m))
)`)
	cons, err := e.LoadComponent(ctx, bytes.NewReader(consBin))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := e.LoadComponent(ctx, bytes.NewReader(provBin))
	if err != nil {
		t.Fatal(err)
	}
	provInst, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mod := provInst.ExportedModule("m")
	if mod == nil {
		t.Fatal("provider did not export m")
	}
	if _, err := cons.Instantiate(ctx, WithModuleImport("m", mod)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestInstantiatedFuncCarriesWasmparserFuncType(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	provSrc := `(component
  (core module $m (func (export "f") (param i32) (result i32) local.get 0))
  (core instance $i (instantiate $m))
  (func (export "f") (param "x" u32) (result u32) (canon lift (core func $i "f")))
)`
	bin := buildComponentBytes(t, provSrc)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fn := inst.ExportedFunc("f")
	if fn == nil {
		t.Fatal("no exported function f")
	}
	if fn.ParserFunctionType() == nil {
		t.Fatal("expected ParserFunctionType on the exported func")
	}
}

func TestInstantiateRejectsModuleArgWithWrongType(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	consBin := buildComponentBytes(t, `(component
  (core type $mt (module (export "f" (func))))
  (import "m" (core module (type $mt)))
)`)
	// Provider's module exports "g" instead of "f" — signature mismatch.
	provBin := buildComponentBytes(t, `(component
  (core module $m (func (export "g")))
  (export "m" (core module $m))
)`)
	cons, err := e.LoadComponent(ctx, bytes.NewReader(consBin))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := e.LoadComponent(ctx, bytes.NewReader(provBin))
	if err != nil {
		t.Fatal(err)
	}
	provInst, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mod := provInst.ExportedModule("m")
	if mod == nil {
		t.Fatal("provider did not export m")
	}
	_, err = cons.Instantiate(ctx, WithModuleImport("m", mod))
	if err == nil {
		t.Fatal("expected mismatch error from pre-check, got nil")
	}
}

func TestInstantiate_ImportEqType(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	f, err := os.Open("testdata/import-eq-type.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)
}
