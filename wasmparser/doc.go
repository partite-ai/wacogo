//go:generate go run ./internal/testgen -wast-dir ./testdata/wast -out-dir ./testdata/generated

// Package wasmparser provides a streaming parser and validator for
// WebAssembly Component Model binaries.
package wasmparser
