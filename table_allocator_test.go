package wacogo_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
)

func TestCoreTableAllocatorPublicAPI(t *testing.T) {
	// One defined table, minimum zero and maximum two; no toolchain required.
	module := []byte{0, 97, 115, 109, 1, 0, 0, 0, 4, 5, 1, 0x70, 1, 0, 2}
	tables, err := wacogo.CoreModuleTables(module)
	if err != nil || len(tables) != 1 || tables[0].Min != 0 || !tables[0].Max.Valid || tables[0].Max.Value != 2 {
		t.Fatalf("public module inspection: tables=%+v err=%v", tables, err)
	}
	data := append([]byte{0, 97, 115, 109, 13, 0, 1, 0, 1, byte(len(module))}, module...)
	data = append(data, 2, 4, 1, 0, 0, 0) // instantiate core module zero
	calls, releases := 0, 0
	ctx := wacogo.WithCoreTableAllocator(t.Context(), func(tables []wacogo.CoreTable) (func(), error) {
		calls++
		if len(tables) != 1 || tables[0].Max.Value != 2 {
			t.Fatalf("callback lost immutable limits: %+v", tables)
		}
		tables[0].Max.Value = 999 // Callback slices must not mutate compiled metadata.
		return func() { releases++ }, nil
	})
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() {
		if err := engine.Close(context.WithoutCancel(ctx)); err != nil {
			t.Error(err)
		}
	})
	component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		instance, err := component.Instantiate(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := instance.Close(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if calls != 2 || releases != 2 {
		t.Fatalf("public callback lifecycle: calls=%d releases=%d", calls, releases)
	}
}
