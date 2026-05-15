package core

import (
	"context"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

func TestNewEngine_WithRuntimeConfig(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExtendedConst).
		WithCloseOnContextDone(true)

	engine := NewEngine(ctx, WithRuntimeConfig(cfg))
	defer engine.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	if _, err := engine.LoadComponent(ctx, f); err != nil {
		t.Fatalf("LoadComponent with custom runtime config: %v", err)
	}
}
