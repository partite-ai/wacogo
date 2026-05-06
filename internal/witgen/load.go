// Package witgen converts a WIT file into a Go bindings package
// targeting the wacogo host layer.
package witgen

import (
	"fmt"

	"go.bytecodealliance.org/wit"
)

// Load reads a WIT text or JSON file from path and returns the
// resolved AST. The wit package delegates WIT-text parsing to a
// vendored wasm-tools; JSON files are decoded directly. `use` and
// `include` references resolve before Load returns, so the *Resolve
// can be walked without further name lookup.
func Load(path string) (*wit.Resolve, error) {
	res, err := wit.LoadWIT(path)
	if err != nil {
		return nil, fmt.Errorf("witgen: load %s: %w", path, err)
	}
	return res, nil
}
