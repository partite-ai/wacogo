package canon

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// testInstance is a minimal canon.Instance for tests that need pointer-
// identity comparison. name gives each instance a distinct non-zero-size
// field so that Go does not assign the same address to distinct allocations
// (the language permits this for zero-size structs). Enter/CanLeave are no-ops.
type testInstance struct{ name string }

// testResourceType is a minimal ResourceType for table tests. Ported from the
// now-deleted resource_test.go.
type testResourceType struct{ name string }

func (*testResourceType) IsResourceType()                                    {}
func (*testResourceType) DefiningInstance() Instance                        { return nil }
func (*testResourceType) Destructor() func(context.Context, uint32) error   { return nil }

func (*testInstance) Enter(ctx context.Context) (func(context.Context), error) {
	return func(context.Context) {}, nil
}
func (*testInstance) CanLeave() bool               { return true }
func (*testInstance) SuspendLeave() func()         { return func() {} }
func (*testInstance) ResourceTable() ResourceTable { return nil }
func (*testInstance) Poison(error)                 {}

// newStubCoreFunc returns an api.Function backed by a real wazero module.
// paramTypes and resultTypes are slices of wasm value-type bytes (e.g.
// wasm.ValI64); pass nil for void. The function body pushes a zero constant
// for each result and returns.
//
// api.Function embeds internalapi.WazeroOnly and cannot be implemented outside
// the wazero module, so a real compiled module is the only portable approach.
func newStubCoreFunc(t *testing.T, paramTypes, resultTypes []byte) api.Function {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })

	var mb wasm.ModuleBuilder

	// Body: push i64.const 0 for each result slot, then end.
	var body []byte
	for range resultTypes {
		body = append(body, 0x42, 0x00) // i64.const 0
	}
	body = append(body, 0x0b) // end

	sig := wasm.FuncSig{Params: paramTypes, Results: resultTypes}
	idx := mb.AddFunc(sig, nil, body)
	mb.AddExportFunc("stub", idx)

	mod, err := r.Instantiate(ctx, mb.Encode())
	if err != nil {
		t.Fatalf("newStubCoreFunc: instantiate: %v", err)
	}
	t.Cleanup(func() { _ = mod.Close(ctx) })

	fn := mod.ExportedFunction("stub")
	if fn == nil {
		t.Fatal("newStubCoreFunc: exported function 'stub' not found")
	}
	return fn
}

// setupMem returns an api.Memory backed by a real wazero runtime with a
// single memory page. A fake in-memory implementation is infeasible here:
// api.Memory embeds internalapi.WazeroOnly, which cannot be implemented
// from outside wazero's module due to Go's internal-package rule. Using a
// real runtime-backed memory is the same pattern used by the top-level
// test helper in lift_lower_test.go.
func setupMem(t *testing.T, size uint32) api.Memory {
	t.Helper()
	_ = size // 1 page (64 KiB) is always allocated; tests fit inside this.
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })

	var mb wasm.ModuleBuilder
	mb.AddMemory(1, 0)
	mb.AddExportMemory("memory", 0)

	mod, err := r.Instantiate(ctx, mb.Encode())
	if err != nil {
		t.Fatalf("failed to instantiate test module: %v", err)
	}
	t.Cleanup(func() { _ = mod.Close(ctx) })

	return mod.Memory()
}
