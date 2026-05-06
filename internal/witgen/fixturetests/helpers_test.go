package fixturetests_test

import (
	"testing"

	"github.com/partite-ai/wacogo/wasmtools"
)

func watToBinary(t *testing.T, wat string) []byte {
	t.Helper()
	ctx := t.Context()
	tool, err := wasmtools.Default(ctx)
	if err != nil {
		t.Fatalf("wasmtools.Default: %v", err)
	}
	out, err := tool.Parse(ctx, []byte(wat))
	if err != nil {
		t.Fatalf("wasm-tools parse: %v", err)
	}
	return out
}
