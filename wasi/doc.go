// Package wasi houses the wacogo bindings for wasi p2.
package wasi

//go:generate go run github.com/partite-ai/wacogo/cmd/wacogo-witgen generate -w wasi:wacogo/imports -o ../internal/wasi/gen -p github.com/partite-ai/wacogo/internal/wasi/gen ../internal/wasi/wit/
