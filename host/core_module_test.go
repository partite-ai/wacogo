package host_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
)

// minimalCoreModuleWasm encodes:
//
//	(module
//	  (func (export "f") (result i32) i32.const 7)
//	  (global (export "g") i32 i32.const 11))
//
// Hand-encoded so the test has no wat2wasm dependency.
var minimalCoreModuleWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f, // type: () -> (i32)
	0x03, 0x02, 0x01, 0x00, // func: type 0
	0x06, 0x06, 0x01, 0x7f, 0x00, 0x41, 0x0b, 0x0b, // global: i32 const 11
	0x07, 0x09, 0x02,
	0x01, 'f', 0x00, 0x00, // export f (func 0)
	0x01, 'g', 0x03, 0x00, // export g (global 0)
	0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x07, 0x0b, // code: f returns 7
}

func TestAddCoreModule_ExportsCompiledModule(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("modtest")
	if err := b.AddCoreModule("simple", minimalCoreModuleWasm); err != nil {
		t.Fatalf("AddCoreModule: %v", err)
	}
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

	cm := inst.Core().ExportedModule("simple")
	if cm == nil {
		t.Fatal("ExportedModule(\"simple\") is nil")
	}
}

func TestAddCoreModule_BadWasm_ErrorsAtBuild(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("modtest_bad")
	if err := b.AddCoreModule("bad", []byte{0xde, 0xad, 0xbe, 0xef}); err == nil {
		t.Fatal("AddCoreModule with malformed wasm: want error, got nil")
	}
}
