package host

import (
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

// TestStub_GrowsMemoryForLargeString exercises the host-stub bump
// allocator with a string argument that exceeds one wasm page
// (64 KiB). Without memory.grow inside the stub's realloc, the
// canonical-ABI lower step writes past the initial 1-page memory
// and traps with an out-of-bounds error.
func TestStub_GrowsMemoryForLargeString(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	ty := &FuncType{
		Params:  []Param{{"s", String}},
		Results: []ResultDecl{{"", U32}},
	}
	b.AddFunction("len", ty, func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		// stack: [ptr, len] -> [resultlen]
		stack[0] = stack[1]
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	// 200 KiB — well past the stub's initial 1-page (64 KiB) memory.
	const n = 200 * 1024
	big := strings.Repeat("a", n)

	results, err := inst.Core().ExportedFunc("len").Call(ctx, core.ValString(big))
	if err != nil {
		t.Fatalf("Call(big string): %v", err)
	}
	if got := uint32(results[0].(core.ValU32)); got != n {
		t.Fatalf("len(big) = %d, want %d", got, n)
	}
}
