package wasm

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func compileModule(t *testing.T, r wazero.Runtime, data []byte) wazero.CompiledModule {
	t.Helper()
	cm, err := r.CompileModule(context.Background(), data)
	if err != nil {
		t.Fatalf("CompileModule failed: %v", err)
	}
	return cm
}

// TestEmptyModule verifies that an empty ModuleBuilder produces a valid wasm module.
func TestEmptyModule(t *testing.T) {
	var b ModuleBuilder
	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())
	cm := compileModule(t, r, data)
	defer cm.Close(context.Background())
}

// TestImportFunc verifies that a module with a function import compiles correctly.
func TestImportFunc(t *testing.T) {
	var b ModuleBuilder
	idx := b.AddImportFunc("env", "log", FuncSig{Params: []byte{ValI32}, Results: nil})
	if idx != 0 {
		t.Fatalf("expected import func index 0, got %d", idx)
	}

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())
	cm := compileModule(t, r, data)
	defer cm.Close(context.Background())

	// Verify the import shows up in the compiled module
	imports := cm.ImportedFunctions()
	if len(imports) != 1 {
		t.Fatalf("expected 1 imported function, got %d", len(imports))
	}
	mod, name, _ := imports[0].Import()
	if mod != "env" || name != "log" {
		t.Fatalf("unexpected import: %q %q", mod, name)
	}
}

// TestImportMemory verifies that a module with a memory import compiles correctly.
func TestImportMemory(t *testing.T) {
	var b ModuleBuilder
	idx := b.AddImportMemory("env", "mem", 1, 4)
	if idx != 0 {
		t.Fatalf("expected import memory index 0, got %d", idx)
	}

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())
	cm := compileModule(t, r, data)
	defer cm.Close(context.Background())

	imports := cm.ImportedMemories()
	if len(imports) != 1 {
		t.Fatalf("expected 1 imported memory, got %d", len(imports))
	}
	mod, name, _ := imports[0].Import()
	if mod != "env" || name != "mem" {
		t.Fatalf("unexpected import: %q %q", mod, name)
	}
}

// TestFuncAndExport verifies that a defined function with an export compiles and is visible.
func TestFuncAndExport(t *testing.T) {
	var b ModuleBuilder
	// i32.const 42, end
	body := []byte{0x41, 42, 0x0b}
	idx := b.AddFunc(FuncSig{Params: nil, Results: []byte{ValI32}}, nil, body)
	b.AddExportFunc("answer", idx)

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())
	cm := compileModule(t, r, data)
	defer cm.Close(context.Background())

	exports := cm.ExportedFunctions()
	if _, ok := exports["answer"]; !ok {
		t.Fatal("expected export 'answer' not found")
	}
}

// TestInstantiateAndCall verifies that an add(i32,i32)->i32 function can be called.
func TestInstantiateAndCall(t *testing.T) {
	var b ModuleBuilder
	// local.get 0, local.get 1, i32.add, end
	body := []byte{0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b}
	idx := b.AddFunc(FuncSig{Params: []byte{ValI32, ValI32}, Results: []byte{ValI32}}, nil, body)
	b.AddExportFunc("add", idx)

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())

	mod, err := r.InstantiateWithConfig(context.Background(), data,
		wazero.NewModuleConfig().WithName("test"))
	if err != nil {
		t.Fatalf("InstantiateWithConfig failed: %v", err)
	}
	defer mod.Close(context.Background())

	fn := mod.ExportedFunction("add")
	if fn == nil {
		t.Fatal("exported function 'add' not found")
	}
	results, err := fn.Call(context.Background(), 7, 35)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if len(results) != 1 || results[0] != 42 {
		t.Fatalf("expected 42, got %v", results)
	}
}

// TestMemoryAndDataSegment verifies that data segments initialize memory correctly.
func TestMemoryAndDataSegment(t *testing.T) {
	var b ModuleBuilder
	b.AddMemory(1, 0)
	b.AddDataSegment(8, []byte("hello"))
	b.AddExportMemory("mem", 0)

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())

	mod, err := r.InstantiateWithConfig(context.Background(), data,
		wazero.NewModuleConfig().WithName("test_mem"))
	if err != nil {
		t.Fatalf("InstantiateWithConfig failed: %v", err)
	}
	defer mod.Close(context.Background())

	mem := mod.ExportedMemory("mem")
	if mem == nil {
		t.Fatal("exported memory 'mem' not found")
	}
	buf, ok := mem.Read(8, 5)
	if !ok {
		t.Fatal("memory read failed")
	}
	if string(buf) != "hello" {
		t.Fatalf("expected 'hello', got %q", buf)
	}
}

// TestLocalEntries verifies that function locals are correctly declared.
func TestLocalEntries(t *testing.T) {
	var b ModuleBuilder
	locals := []LocalEntry{NewLocalEntry(2, ValI32)}
	// i32.const 10, local.set 2, local.get 2, end
	body := []byte{0x41, 10, 0x21, 0x02, 0x20, 0x02, 0x0b}
	idx := b.AddFunc(FuncSig{Params: []byte{ValI32, ValI32}, Results: []byte{ValI32}}, locals, body)
	b.AddExportFunc("fn", idx)

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())

	mod, err := r.InstantiateWithConfig(context.Background(), data,
		wazero.NewModuleConfig().WithName("test_locals"))
	if err != nil {
		t.Fatalf("InstantiateWithConfig failed: %v", err)
	}
	defer mod.Close(context.Background())

	fn := mod.ExportedFunction("fn")
	if fn == nil {
		t.Fatal("exported function 'fn' not found")
	}
	results, err := fn.Call(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if len(results) != 1 || api.DecodeI32(results[0]) != 10 {
		t.Fatalf("expected 10, got %v", results)
	}
}

// TestGlobal verifies that a mutable global can be defined, exported, read and written.
func TestGlobal(t *testing.T) {
	var b ModuleBuilder
	globalIdx := b.AddGlobal(ValI32, true, 1024)
	if globalIdx != 0 {
		t.Fatalf("expected global index 0, got %d", globalIdx)
	}

	// Function that reads the global, adds 1, writes it back, and returns old value.
	var code CodeBuilder
	code.GlobalGet(0)     // push old value
	code.GlobalGet(0)     // push old value again
	code.I32Const(1)      // push 1
	code.I32Add()         // old + 1
	code.GlobalSet(0)     // write new value
	code.End()            // return old value (still on stack)

	funcIdx := b.AddFunc(
		FuncSig{Params: nil, Results: []byte{ValI32}},
		nil,
		code.Bytes(),
	)
	b.AddExportFunc("inc", funcIdx)
	b.AddMemory(1, 0) // memory needed for a valid module with data

	data := b.Encode()
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())

	mod, err := r.InstantiateWithConfig(context.Background(), data,
		wazero.NewModuleConfig().WithName("test_global"))
	if err != nil {
		t.Fatalf("InstantiateWithConfig failed: %v", err)
	}
	defer mod.Close(context.Background())

	fn := mod.ExportedFunction("inc")
	if fn == nil {
		t.Fatal("exported function 'inc' not found")
	}

	// First call: should return 1024 (initial value)
	results, err := fn.Call(context.Background())
	if err != nil {
		t.Fatalf("Call 1 failed: %v", err)
	}
	if len(results) != 1 || api.DecodeI32(results[0]) != 1024 {
		t.Fatalf("expected 1024, got %v", results)
	}

	// Second call: should return 1025
	results, err = fn.Call(context.Background())
	if err != nil {
		t.Fatalf("Call 2 failed: %v", err)
	}
	if len(results) != 1 || api.DecodeI32(results[0]) != 1025 {
		t.Fatalf("expected 1025, got %v", results)
	}
}
