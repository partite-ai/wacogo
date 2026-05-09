// jsinterp loads a wrapper WASI component and uses it to
// evaluate a JavaScript file passed on the command line.
//
// Usage:
//
//	jsinterp <script.js>
//
// The wrapper component is embedded into the binary via go:embed.
// At runtime the script file is exposed to the guest under a virtual
// filesystem rooted at "/".
package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"testing/fstest"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/wasi"
	"github.com/partite-ai/wacogo/wasi/filesystem/preopens"
)

//go:embed wrapper/wrapper.wasm
var wrapperWasm []byte

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <script.js>", os.Args[0])
	}
	scriptPath := os.Args[1]

	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		log.Fatalf("read %s: %v", scriptPath, err)
	}

	scriptFS := fstest.MapFS{
		"script.js": &fstest.MapFile{Data: scriptBytes},
	}

	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	w, err := wasi.NewWorld(ctx, e, &wasi.Config{
		Args:     []string{},
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Preopens: preopens.NewFSPreopens(preopens.ImmutableFS{FS: scriptFS}),
	})
	if err != nil {
		log.Fatalf("build wasi world: %v", err)
	}
	defer w.Close(ctx)

	comp, err := e.LoadComponent(ctx, bytes.NewReader(wrapperWasm))
	if err != nil {
		log.Fatalf("load wrapper.wasm: %v", err)
	}

	opts := w.Imports()

	inst, err := comp.Instantiate(ctx, opts...)
	if err != nil {
		log.Fatalf("instantiate wrapper.wasm: %v", err)
	}
	defer inst.Close(ctx)

	runFn := inst.ExportedFunc("eval-path")
	if runFn == nil {
		log.Fatal(`component does not export "eval-path" function`)
	}

	results, err := runFn.Call(ctx, wacogo.ValString("/script.js"))
	if err != nil {
		log.Fatalf("call eval-path: %v", err)
	}
	if len(results) != 1 {
		log.Fatalf("expected 1 result from eval-path, got %d", len(results))
	}

	r, ok := results[0].(*wacogo.ValResult)
	if !ok {
		log.Fatalf("expected result<_,_> from eval-file, got %T", results[0])
	}
	if !r.IsOk() {
		fmt.Fprintln(os.Stderr, "script exited with error", r.Err())
		os.Exit(1)
	}
}
