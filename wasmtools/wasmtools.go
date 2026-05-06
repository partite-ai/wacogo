// Package wasmtools embeds the wasip1 build of bytecodealliance/wasm-tools
// and runs it inside wazero, so callers can use wasm-tools without
// requiring the binary to be installed on the host.
//
// Construct one Tool per process with New and reuse it across calls;
// compilation of the embedded binary is expensive. For test helpers,
// Default returns a process-wide singleton.
//
// Regenerate the embedded binary with:
//
//	go generate ./wasmtools/...
package wasmtools

//go:generate go run fetch.go

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

//go:embed wasm-tools.wasm.gz
var wasmToolsGz []byte

var (
	decompressOnce sync.Once
	wasmToolsBin   []byte
	decompressErr  error
)

func decompress() ([]byte, error) {
	decompressOnce.Do(func() {
		gr, err := gzip.NewReader(bytes.NewReader(wasmToolsGz))
		if err != nil {
			decompressErr = err
			return
		}
		defer gr.Close()
		wasmToolsBin, decompressErr = io.ReadAll(gr)
	})
	return wasmToolsBin, decompressErr
}

// Tool holds a wazero runtime with the wasm-tools binary precompiled.
// Reuse a single Tool across calls; compilation is expensive.
type Tool struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

// New decompresses the embedded wasm-tools binary, builds a wazero
// runtime, and compiles it. Close releases both.
func New(ctx context.Context) (*Tool, error) {
	bin, err := decompress()
	if err != nil {
		return nil, fmt.Errorf("wasmtools: decompress binary: %w", err)
	}
	rt := wazero.NewRuntime(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("wasmtools: instantiate WASI: %w", err)
	}
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("wasmtools: compile wasm-tools: %w", err)
	}
	return &Tool{runtime: rt, compiled: compiled}, nil
}

// Close releases the underlying wazero runtime.
func (t *Tool) Close(ctx context.Context) error {
	return t.runtime.Close(ctx)
}

type runConfig struct {
	args   []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	fs     wazero.FSConfig
}

func (t *Tool) run(ctx context.Context, cfg runConfig) error {
	mc := wazero.NewModuleConfig().
		WithName("").
		WithArgs(append([]string{"wasm-tools"}, cfg.args...)...).
		WithStdin(cfg.stdin).
		WithStdout(cfg.stdout).
		WithStderr(cfg.stderr).
		WithStartFunctions("_start")
	if cfg.fs != nil {
		mc = mc.WithFSConfig(cfg.fs)
	}
	mod, err := t.runtime.InstantiateModule(ctx, t.compiled, mc)
	if mod != nil {
		_ = mod.Close(ctx)
	}
	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 0 {
				return nil
			}
			return fmt.Errorf("wasm-tools %s: exit %d", strings.Join(cfg.args, " "), exitErr.ExitCode())
		}
		return fmt.Errorf("wasm-tools %s: %w", strings.Join(cfg.args, " "), err)
	}
	return nil
}

var (
	defaultOnce sync.Once
	defaultTool *Tool
	defaultErr  error
)

// Default returns a process-wide Tool, constructing it on first call.
// The returned Tool is never closed — the wazero runtime leaks for the
// lifetime of the process. Suitable for tests and one-shot CLI tools
// that share an instance across many invocations.
func Default(ctx context.Context) (*Tool, error) {
	defaultOnce.Do(func() { defaultTool, defaultErr = New(ctx) })
	return defaultTool, defaultErr
}

// Parse converts WAT text into a wasm binary via `wasm-tools parse -`.
func (t *Tool) Parse(ctx context.Context, wat []byte) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	err := t.run(ctx, runConfig{
		args:   []string{"parse", "-"},
		stdin:  bytes.NewReader(wat),
		stdout: &stdout,
		stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("%w\n%s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// JsonFromWast runs `wasm-tools json-from-wast --pretty` against the
// .wast file at wastPath, writing <stem>.json and a <stem>/ directory
// of per-fixture .wasm files into outDir. wastPath and outDir are
// host-filesystem paths; the package mounts read-only and read-write
// preopens around them.
func (t *Tool) JsonFromWast(ctx context.Context, wastPath, outDir string) error {
	absWast, err := filepath.Abs(wastPath)
	if err != nil {
		return fmt.Errorf("wasmtools: resolve wastPath: %w", err)
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("wasmtools: resolve outDir: %w", err)
	}
	wastDir, wastFile := filepath.Split(absWast)
	stem := strings.TrimSuffix(wastFile, ".wast")
	if err := os.MkdirAll(filepath.Join(absOut, stem), 0o755); err != nil {
		return fmt.Errorf("wasmtools: create wasm dir: %w", err)
	}

	fsCfg := wazero.NewFSConfig().
		WithReadOnlyDirMount(filepath.Clean(wastDir), "/in").
		WithDirMount(absOut, "/out")

	var stderr bytes.Buffer
	if err := t.run(ctx, runConfig{
		args: []string{
			"json-from-wast",
			"--pretty",
			"--wasm-dir", "/out/" + stem,
			"-o", "/out/" + stem + ".json",
			"/in/" + wastFile,
		},
		stderr: &stderr,
		fs:     fsCfg,
	}); err != nil {
		return fmt.Errorf("%w\n%s", err, stderr.String())
	}
	return nil
}
