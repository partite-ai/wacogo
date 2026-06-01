package host_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/wasmtools"
)

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func benchParse(b *testing.B, wat string) []byte {
	b.Helper()
	tool, err := wasmtools.Default(b.Context())
	if err != nil {
		b.Fatalf("wasmtools.Default: %v", err)
	}
	out, err := tool.Parse(b.Context(), []byte(wat))
	if err != nil {
		b.Fatalf("wasm-tools parse:\n%s\nerr: %v", wat, err)
	}
	return out
}

func benchEngine(b *testing.B) *wacogo.Engine {
	b.Helper()
	e := wacogo.NewEngine(b.Context())
	b.Cleanup(func() { _ = e.Close(context.Background()) })
	return e
}

func benchLoad(b *testing.B, e *wacogo.Engine, wat string) *wacogo.Component {
	b.Helper()
	comp, err := e.LoadComponent(b.Context(), bytes.NewReader(benchParse(b, wat)))
	if err != nil {
		b.Fatalf("LoadComponent:\n%s\nerr: %v", wat, err)
	}
	return comp
}

func benchInstantiate(b *testing.B, comp *wacogo.Component, opts ...wacogo.InstantiateOption) *wacogo.ComponentInstance {
	b.Helper()
	inst, err := comp.Instantiate(b.Context(), opts...)
	if err != nil {
		b.Fatalf("Instantiate: %v", err)
	}
	b.Cleanup(func() { _ = inst.Close(context.Background()) })
	return inst
}

// benchHostComponent builds and instantiates a host component declaring a
// single function export `f` with signature ty backed by impl.
func benchHostComponent(b *testing.B, e *wacogo.Engine, ty *host.FuncType, impl host.Func) *host.ComponentInstance {
	b.Helper()
	hb := e.NewHostBuilder("bench-host")
	hb.AddFunction("f", ty, impl)
	hc, err := hb.Build(b.Context())
	if err != nil {
		b.Fatalf("host Build: %v", err)
	}
	b.Cleanup(func() { _ = hc.Close(context.Background()) })
	inst, err := hc.Instantiate(b.Context())
	if err != nil {
		b.Fatalf("host Instantiate: %v", err)
	}
	b.Cleanup(func() { _ = inst.Close(context.Background()) })
	return inst
}

// ----------------------------------------------------------------------------
// WAT fixtures
//
// Convention: every "callee" component exports `f` with the signature under
// test. Every "caller" component imports `f` and exports `call_f` that
// forwards through canon lower + wasm call + canon lift. The caller fixture is
// reused for both Comp→Host and Comp→Comp: the only difference is what
// satisfies the `f` import at Instantiate time.
// ----------------------------------------------------------------------------

const watNoargCallee = `(component
  (core module $m (func (export "f")))
  (core instance $i (instantiate $m))
  (func (export "f") (canon lift (core func $i "f")))
)`

// Comp→Host and Comp→Comp benchmarks export a `bench(n: u32)` function that
// loops n times calling imported $f. The Go-side test harness invokes
// `bench(b.N)` exactly once per benchmark execution, amortising all
// Go-side Func.Call cost over b.N inner cross-component calls. This isolates
// the pure wasm→wasm (or wasm→host) per-call overhead; allocs/op and
// ns/op reported by `go test -bench` reflect only that.

const watNoargCaller = `(component
  (import "f" (func $f))
  (core func $lf (canon lower (func $f)))
  (core module $m
    (import "" "f" (func $f))
    (func (export "bench") (param $n i32)
      (block $exit
        (br_if $exit (i32.eqz (local.get $n)))
        (loop $loop
          (call $f)
          (local.set $n (i32.sub (local.get $n) (i32.const 1)))
          (br_if $loop (local.get $n))))))
  (core instance $bridge (export "f" (func $lf)))
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "bench" (core func $cbench))
  (type $bench-ft (func (param "n" u32)))
  (func (export "bench") (type $bench-ft) (canon lift (core func $cbench)))
)`

const watPrimCallee = `(component
  (core module $m
    (func (export "f") (param i32 i32) (result i32)
      local.get 0 local.get 1 i32.add))
  (core instance $i (instantiate $m))
  (func (export "f") (param "a" u32) (param "b" u32) (result u32) (canon lift (core func $i "f")))
)`

const watPrimCaller = `(component
  (type $imp (func (param "a" u32) (param "b" u32) (result u32)))
  (import "f" (func $f (type $imp)))
  (core func $lf (canon lower (func $f)))
  (core module $m
    (import "" "f" (func $f (param i32 i32) (result i32)))
    (func (export "bench") (param $n i32) (result i32)
      (local $acc i32)
      (block $exit
        (br_if $exit (i32.eqz (local.get $n)))
        (loop $loop
          (local.set $acc
            (i32.add (local.get $acc)
              (call $f (i32.const 1) (i32.const 2))))
          (local.set $n (i32.sub (local.get $n) (i32.const 1)))
          (br_if $loop (local.get $n))))
      (local.get $acc)))
  (core instance $bridge (export "f" (func $lf)))
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "bench" (core func $cbench))
  (type $bench-ft (func (param "n" u32) (result u32)))
  (func (export "bench") (type $bench-ft) (canon lift (core func $cbench)))
)`

// watBumpRealloc is a reusable bump allocator core module body, exporting
// memory, cabi_realloc, and post_return_i32 (a post-return hook that resets
// the bump pointer between calls so benchmark iterations don't exhaust
// linear memory). post_return_i32 fits any signature whose flat return is a
// single i32 (any single scalar result, or any mem-result indirect ptr).
const watBumpRealloc = `
    (memory (export "memory") 1)
    (global $bump (mut i32) (i32.const 64))
    (func $realloc (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr))
    (func (export "post-return-i32") (param i32)
      (global.set $bump (i32.const 64)))`

// String signature: (string) -> u32 (returns byte length of input).
// Exercises encodeString on the param lower path; the comp→comp variant
// additionally exercises transferStringContent on the cross-component edge.

const watStringCallee = `(component
  (core module $m` + watBumpRealloc + `
    (func (export "f") (param i32 i32) (result i32)
      local.get 1))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "cabi_realloc" (core func $realloc))
  (alias core export $i "post-return-i32" (core func $postret))
  (alias core export $i "f" (core func $cf))
  (type $ft (func (param "s" string) (result u32)))
  (func (export "f") (type $ft) (canon lift (core func $cf) (memory $mem) (realloc $realloc) (post-return $postret)))
)`

// stringCallerWAT returns a bench-loop caller component for the (string) ->
// u32 signature. The static buffer holds `maxSize` bytes of 'a'; the bench
// export takes (n, size) and loops n times calling imported $f with a
// string view of the first `size` bytes of that buffer.
func stringCallerWAT(maxSize int) string {
	bumpStart := maxSize + 16
	return fmt.Sprintf(`(component
  (type $imp (func (param "s" string) (result u32)))
  (import "f" (func $f (type $imp)))
  (core module $mm
    (memory (export "memory") 1)
    (data (i32.const 0) "%s")
    (global $bump (mut i32) (i32.const %d))
    (func (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr)))
  (core instance $mi (instantiate $mm))
  (alias core export $mi "memory" (core memory $mem))
  (alias core export $mi "cabi_realloc" (core func $realloc))
  (core func $lf (canon lower (func $f) (memory $mem) (realloc $realloc)))
  (core module $main
    (import "env" "memory" (memory 1))
    (import "env" "f" (func $f (param i32 i32) (result i32)))
    (func (export "bench") (param $n i32) (param $size i32) (result i32)
      (local $acc i32)
      (block $exit
        (br_if $exit (i32.eqz (local.get $n)))
        (loop $loop
          (local.set $acc
            (i32.add (local.get $acc)
              (call $f (i32.const 0) (local.get $size))))
          (local.set $n (i32.sub (local.get $n) (i32.const 1)))
          (br_if $loop (local.get $n))))
      (local.get $acc)))
  (core instance $env (export "memory" (memory $mem))
                      (export "f" (func $lf)))
  (core instance $i (instantiate $main (with "env" (instance $env))))
  (alias core export $i "bench" (core func $cbench))
  (type $bench-ft (func (param "n" u32) (param "size" u32) (result u32)))
  (func (export "bench") (type $bench-ft) (canon lift (core func $cbench)))
)`, strings.Repeat("a", maxSize), bumpStart)
}

// list_record signature: (list<tuple<u32, u32>>) -> u32
// Returns the input list length. Exercises composite alloc patterns on lift —
// every tuple element triggers a fresh *ValRecord plus a []Field slice
// (audit issue #9). Tuple chosen over record to avoid named-type-export
// requirements; the canon lift path is identical for both (both produce
// *ValRecord on the Go side).

const watListRecordCallee = `(component
  (type $t (tuple u32 u32))
  (type $l (list $t))
  (type $ft (func (param "xs" $l) (result u32)))
  (core module $m` + watBumpRealloc + `
    (func (export "f") (param i32 i32) (result i32)
      local.get 1))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "cabi_realloc" (core func $realloc))
  (alias core export $i "post-return-i32" (core func $postret))
  (alias core export $i "f" (core func $cf))
  (func (export "f") (type $ft) (canon lift (core func $cf) (memory $mem) (realloc $realloc) (post-return $postret)))
)`

// listRecordCallerWAT returns a bench-loop caller for the
// (list<tuple<u32, u32>>) -> u32 signature. The list elements live at ptr=0
// in zero-initialised wasm memory (so each tuple is (0, 0)) — the content
// doesn't matter for the per-call cost we're measuring.
const listRecordCallerWAT = `(component
  (type $t (tuple u32 u32))
  (type $l (list $t))
  (type $imp (func (param "xs" $l) (result u32)))
  (import "f" (func $f (type $imp)))
  (core module $mm
    (memory (export "memory") 1)
    (global $bump (mut i32) (i32.const 4096))
    (func (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr)))
  (core instance $mi (instantiate $mm))
  (alias core export $mi "memory" (core memory $mem))
  (alias core export $mi "cabi_realloc" (core func $realloc))
  (core func $lf (canon lower (func $f) (memory $mem) (realloc $realloc)))
  (core module $main
    (import "env" "memory" (memory 1))
    (import "env" "f" (func $f (param i32 i32) (result i32)))
    (func (export "bench") (param $n i32) (param $size i32) (result i32)
      (local $acc i32)
      (block $exit
        (br_if $exit (i32.eqz (local.get $n)))
        (loop $loop
          (local.set $acc
            (i32.add (local.get $acc)
              (call $f (i32.const 0) (local.get $size))))
          (local.set $n (i32.sub (local.get $n) (i32.const 1)))
          (br_if $loop (local.get $n))))
      (local.get $acc)))
  (core instance $env (export "memory" (memory $mem))
                      (export "f" (func $lf)))
  (core instance $i (instantiate $main (with "env" (instance $env))))
  (alias core export $i "bench" (core func $cbench))
  (type $bench-ft (func (param "n" u32) (param "size" u32) (result u32)))
  (func (export "bench") (type $bench-ft) (canon lift (core func $cbench)))
)`

// variant signature: (option<u32>) -> u32
// Returns the option discriminant (0=none, 1=some). Exercises the
// discriminant-dispatch path (visitor_*_to_val.go VisitOption); option uses
// a 2-arm specialization of the variant code path but exercises the same
// switch-on-disc dispatch the audit flagged. Full nominal variants require
// type-export plumbing — deferred to a follow-up.

const watVariantCallee = `(component
  (type $o (option u32))
  (type $ft (func (param "x" $o) (result u32)))
  (core module $m` + watBumpRealloc + `
    (func (export "f") (param i32 i32) (result i32)
      local.get 0))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "cabi_realloc" (core func $realloc))
  (alias core export $i "post-return-i32" (core func $postret))
  (alias core export $i "f" (core func $cf))
  (func (export "f") (type $ft) (canon lift (core func $cf) (memory $mem) (realloc $realloc) (post-return $postret)))
)`

// variantCallerWAT is a bench-loop caller for the (option<u32>) -> u32
// signature. option<u32> flattens to (disc: i32, payload: i32) in canon
// flat mode; the inner loop passes (1, 42) on every iteration.
const variantCallerWAT = `(component
  (type $o (option u32))
  (type $imp (func (param "x" $o) (result u32)))
  (import "f" (func $f (type $imp)))
  (core func $lf (canon lower (func $f)))
  (core module $m
    (import "" "f" (func $f (param i32 i32) (result i32)))
    (func (export "bench") (param $n i32) (result i32)
      (local $acc i32)
      (block $exit
        (br_if $exit (i32.eqz (local.get $n)))
        (loop $loop
          (local.set $acc
            (i32.add (local.get $acc)
              (call $f (i32.const 1) (i32.const 42))))
          (local.set $n (i32.sub (local.get $n) (i32.const 1)))
          (br_if $loop (local.get $n))))
      (local.get $acc)))
  (core instance $bridge (export "f" (func $lf)))
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "bench" (core func $cbench))
  (type $bench-ft (func (param "n" u32) (result u32)))
  (func (export "bench") (type $bench-ft) (canon lift (core func $cbench)))
)`

// ----------------------------------------------------------------------------
// noarg: () -> ()
// ----------------------------------------------------------------------------

func BenchmarkHostToComp_Noarg(b *testing.B) {
	e := benchEngine(b)
	inst := benchInstantiate(b, benchLoad(b, e, watNoargCallee))
	fn := inst.ExportedFunc("f")
	if fn == nil {
		b.Fatal("missing export f")
	}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := fn.Call(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompToHost_Noarg(b *testing.B) {
	e := benchEngine(b)
	hostInst := benchHostComponent(b, e, &host.FuncType{},
		func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })

	caller := benchLoad(b, e, watNoargCaller)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", hostInst.Core().ExportedFunc("f")))
	runBenchLoop(b, callerInst)
}

func BenchmarkCompToComp_Noarg(b *testing.B) {
	e := benchEngine(b)
	calleeInst := benchInstantiate(b, benchLoad(b, e, watNoargCallee))

	caller := benchLoad(b, e, watNoargCaller)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", calleeInst.ExportedFunc("f")))
	runBenchLoop(b, callerInst)
}

// runBenchLoop invokes the caller component's `bench(n: u32)` export once
// with n=b.N, having the wasm side perform b.N inner cross-component calls.
// All Go-side Func.Call overhead (Val construction, results slice, etc.)
// is paid once per benchmark execution and amortised over b.N reported ops.
func runBenchLoop(b *testing.B, callerInst *wacogo.ComponentInstance, extraArgs ...core.Val) {
	b.Helper()
	fn := callerInst.ExportedFunc("bench")
	if fn == nil {
		b.Fatal("missing export bench")
	}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	args := make([]core.Val, 1+len(extraArgs))
	args[0] = core.ValU32(uint32(b.N))
	copy(args[1:], extraArgs)
	if _, err := fn.Call(ctx, args...); err != nil {
		b.Fatal(err)
	}
}

// ----------------------------------------------------------------------------
// prim: (u32, u32) -> u32
// ----------------------------------------------------------------------------

var primArgs = []core.Val{core.ValU32(1), core.ValU32(2)}

func BenchmarkHostToComp_Prim(b *testing.B) {
	e := benchEngine(b)
	inst := benchInstantiate(b, benchLoad(b, e, watPrimCallee))
	fn := inst.ExportedFunc("f")
	if fn == nil {
		b.Fatal("missing export f")
	}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := fn.Call(ctx, primArgs...); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompToHost_Prim(b *testing.B) {
	e := benchEngine(b)
	ty := &host.FuncType{
		Params:  []host.Param{{Name: "a", Type: host.U32}, {Name: "b", Type: host.U32}},
		Results: []host.ResultDecl{{Type: host.U32}},
	}
	hostInst := benchHostComponent(b, e, ty,
		func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
			stack[0] = stack[0] + stack[1]
			return nil
		})

	caller := benchLoad(b, e, watPrimCaller)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", hostInst.Core().ExportedFunc("f")))
	runBenchLoop(b, callerInst)
}

func BenchmarkCompToComp_Prim(b *testing.B) {
	e := benchEngine(b)
	calleeInst := benchInstantiate(b, benchLoad(b, e, watPrimCallee))

	caller := benchLoad(b, e, watPrimCaller)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", calleeInst.ExportedFunc("f")))
	runBenchLoop(b, callerInst)
}

// ----------------------------------------------------------------------------
// string: (string) -> u32 (returns input length)
//
// Subtests vary input size to separate per-call overhead from per-byte
// encoding cost.
// ----------------------------------------------------------------------------

var stringSizes = []int{1, 64, 1024}

func makeStr(n int) core.Val {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = 'a'
	}
	return core.ValString(string(buf))
}

func benchStringCall(b *testing.B, fn *core.ExportedFunc) {
	b.Helper()
	ctx := b.Context()
	for _, n := range stringSizes {
		arg := makeStr(n)
		b.Run("n="+itoa(n), func(b *testing.B) {
			args := []core.Val{arg}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := fn.Call(ctx, args...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkHostToComp_String(b *testing.B) {
	e := benchEngine(b)
	inst := benchInstantiate(b, benchLoad(b, e, watStringCallee))
	fn := inst.ExportedFunc("f")
	if fn == nil {
		b.Fatal("missing export f")
	}
	benchStringCall(b, fn)
}

const maxStringSize = 1024

func BenchmarkCompToHost_String(b *testing.B) {
	e := benchEngine(b)
	ty := &host.FuncType{
		Params:  []host.Param{{Name: "s", Type: host.String}},
		Results: []host.ResultDecl{{Type: host.U32}},
	}
	hostInst := benchHostComponent(b, e, ty,
		func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
			stack[0] = stack[1]
			return nil
		})

	caller := benchLoad(b, e, stringCallerWAT(maxStringSize))
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", hostInst.Core().ExportedFunc("f")))
	runSizedBenchLoop(b, callerInst, stringSizes)
}

func BenchmarkCompToComp_String(b *testing.B) {
	e := benchEngine(b)
	calleeInst := benchInstantiate(b, benchLoad(b, e, watStringCallee))

	caller := benchLoad(b, e, stringCallerWAT(maxStringSize))
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", calleeInst.ExportedFunc("f")))
	runSizedBenchLoop(b, callerInst, stringSizes)
}

// runSizedBenchLoop is the size-sweep variant of runBenchLoop: for each
// element of sizes, invokes bench(n=b.N, size=s) once.
func runSizedBenchLoop(b *testing.B, callerInst *wacogo.ComponentInstance, sizes []int) {
	b.Helper()
	for _, size := range sizes {
		b.Run("size="+itoa(size), func(b *testing.B) {
			runBenchLoop(b, callerInst, core.ValU32(uint32(size)))
		})
	}
}

// ----------------------------------------------------------------------------
// list_record: (list<record{a:u32, b:u32}>) -> u32 (returns input length)
//
// Exercises composite alloc patterns on lift — every record element triggers
// a fresh ValRecord, []Field, plus the lazy field-index map.
// ----------------------------------------------------------------------------

var listSizes = []int{1, 16, 256}

func makeListRecord(n int) core.Val {
	// Tuple fields are positionally named "0", "1", ... per canonical-ABI
	// convention; matches what the canon lift produces.
	records := make([]*core.ValRecord, n)
	for i := range records {
		records[i] = core.NewValRecord(
			core.Field{Name: "0", Val: core.ValU32(uint32(i))},
			core.Field{Name: "1", Val: core.ValU32(uint32(i + 1))},
		)
	}
	return core.NewValListOf(records...)
}

func benchListCall(b *testing.B, fn *core.ExportedFunc) {
	b.Helper()
	ctx := b.Context()
	for _, n := range listSizes {
		arg := makeListRecord(n)
		b.Run("n="+itoa(n), func(b *testing.B) {
			args := []core.Val{arg}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := fn.Call(ctx, args...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkHostToComp_ListRecord(b *testing.B) {
	e := benchEngine(b)
	inst := benchInstantiate(b, benchLoad(b, e, watListRecordCallee))
	fn := inst.ExportedFunc("f")
	if fn == nil {
		b.Fatal("missing export f")
	}
	benchListCall(b, fn)
}

func BenchmarkCompToHost_ListRecord(b *testing.B) {
	e := benchEngine(b)
	hb := e.NewHostBuilder("bench-host")
	tupRef := hb.AddType("xy-tuple", host.Tuple{Types: []host.TypeExpr{host.U32, host.U32}})
	hb.AddFunction("f", &host.FuncType{
		Params:  []host.Param{{Name: "xs", Type: host.List{Elem: tupRef}}},
		Results: []host.ResultDecl{{Type: host.U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		stack[0] = stack[1]
		return nil
	})
	hc, err := hb.Build(b.Context())
	if err != nil {
		b.Fatalf("host Build: %v", err)
	}
	b.Cleanup(func() { _ = hc.Close(context.Background()) })
	hostInst, err := hc.Instantiate(b.Context())
	if err != nil {
		b.Fatalf("host Instantiate: %v", err)
	}
	b.Cleanup(func() { _ = hostInst.Close(context.Background()) })

	caller := benchLoad(b, e, listRecordCallerWAT)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", hostInst.Core().ExportedFunc("f")))
	runSizedBenchLoop(b, callerInst, listSizes)
}

func BenchmarkCompToComp_ListRecord(b *testing.B) {
	e := benchEngine(b)
	calleeInst := benchInstantiate(b, benchLoad(b, e, watListRecordCallee))

	caller := benchLoad(b, e, listRecordCallerWAT)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", calleeInst.ExportedFunc("f")))
	runSizedBenchLoop(b, callerInst, listSizes)
}

// ----------------------------------------------------------------------------
// list_string: (list<string>) -> u32 — non-bulk path
//
// list<string> can't take the bulk-memcpy path because each string element
// needs pointer/length translation, UTF-8 validation, and a separate callee
// realloc. This isolates the per-element-step cost: for size=N the inner
// transfer does N realloc calls + N memcpy operations + N validation passes.
// ----------------------------------------------------------------------------

const watListStringCallee = `(component
  (core module $m
    (memory (export "memory") 1)
    (global $bump (mut i32) (i32.const 64))
    (func $realloc (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr))
    (func (export "post-return-i32") (param i32)
      (global.set $bump (i32.const 64)))
    (func (export "f") (param i32 i32) (result i32)
      local.get 1))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "cabi_realloc" (core func $realloc))
  (alias core export $i "post-return-i32" (core func $postret))
  (alias core export $i "f" (core func $cf))
  (type $ft (func (param "xs" (list string)) (result u32)))
  (func (export "f") (type $ft) (canon lift (core func $cf) (memory $mem) (realloc $realloc) (post-return $postret)))
)`

// listStringCallerWAT lays out a list-of-strings in caller memory:
// element array at offset 0 (each entry is (ptr=2048, len=16)), and the
// shared 16-byte "aaaa..." backing buffer at offset 2048. The bench
// function re-initialises the element array (cheap, amortised) then loops
// n times calling imported $f(list_ptr=0, list_len).
const watListStringCallerWAT = `(component
  (type $imp (func (param "xs" (list string)) (result u32)))
  (import "f" (func $f (type $imp)))
  (core module $mm
    (memory (export "memory") 1)
    (data (i32.const 2048) "aaaaaaaaaaaaaaaa")
    (global $bump (mut i32) (i32.const 4096))
    (func (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr)))
  (core instance $mi (instantiate $mm))
  (alias core export $mi "memory" (core memory $mem))
  (alias core export $mi "cabi_realloc" (core func $realloc))
  (core func $lf (canon lower (func $f) (memory $mem) (realloc $realloc)))
  (core module $main
    (import "env" "memory" (memory 1))
    (import "env" "f" (func $f (param i32 i32) (result i32)))
    (func (export "bench") (param $n i32) (param $listLen i32) (result i32)
      (local $i i32)
      (local $off i32)
      (local $acc i32)
      (block $exit_init
        (br_if $exit_init (i32.eqz (local.get $listLen)))
        (loop $init
          (i32.store (local.get $off) (i32.const 2048))
          (i32.store offset=4 (local.get $off) (i32.const 16))
          (local.set $off (i32.add (local.get $off) (i32.const 8)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br_if $init (i32.lt_u (local.get $i) (local.get $listLen)))))
      (block $exit_main
        (br_if $exit_main (i32.eqz (local.get $n)))
        (loop $main
          (local.set $acc (i32.add (local.get $acc) (call $f (i32.const 0) (local.get $listLen))))
          (local.set $n (i32.sub (local.get $n) (i32.const 1)))
          (br_if $main (local.get $n))))
      (local.get $acc)))
  (core instance $env (export "memory" (memory $mem))
                      (export "f" (func $lf)))
  (core instance $i (instantiate $main (with "env" (instance $env))))
  (alias core export $i "bench" (core func $cbench))
  (type $bench-ft (func (param "n" u32) (param "list-len" u32) (result u32)))
  (func (export "bench") (type $bench-ft) (canon lift (core func $cbench)))
)`

func BenchmarkCompToHost_ListString(b *testing.B) {
	e := benchEngine(b)
	hb := e.NewHostBuilder("bench-host")
	hb.AddFunction("f", &host.FuncType{
		Params:  []host.Param{{Name: "xs", Type: host.List{Elem: host.String}}},
		Results: []host.ResultDecl{{Type: host.U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		stack[0] = stack[1]
		return nil
	})
	hc, err := hb.Build(b.Context())
	if err != nil {
		b.Fatalf("host Build: %v", err)
	}
	b.Cleanup(func() { _ = hc.Close(context.Background()) })
	hostInst, err := hc.Instantiate(b.Context())
	if err != nil {
		b.Fatalf("host Instantiate: %v", err)
	}
	b.Cleanup(func() { _ = hostInst.Close(context.Background()) })

	caller := benchLoad(b, e, watListStringCallerWAT)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", hostInst.Core().ExportedFunc("f")))
	runSizedBenchLoop(b, callerInst, listSizes)
}

func BenchmarkCompToComp_ListString(b *testing.B) {
	e := benchEngine(b)
	calleeInst := benchInstantiate(b, benchLoad(b, e, watListStringCallee))

	caller := benchLoad(b, e, watListStringCallerWAT)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", calleeInst.ExportedFunc("f")))
	runSizedBenchLoop(b, callerInst, listSizes)
}

// ----------------------------------------------------------------------------
// variant: (option<u32>) -> u32 (returns 1 for some, 0 for none)
// ----------------------------------------------------------------------------

var variantArgs = []core.Val{core.ValOptionSome(core.ValU32(42))}

func BenchmarkHostToComp_Variant(b *testing.B) {
	e := benchEngine(b)
	inst := benchInstantiate(b, benchLoad(b, e, watVariantCallee))
	fn := inst.ExportedFunc("f")
	if fn == nil {
		b.Fatal("missing export f")
	}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := fn.Call(ctx, variantArgs...); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompToHost_Variant(b *testing.B) {
	e := benchEngine(b)
	ty := &host.FuncType{
		Params:  []host.Param{{Name: "x", Type: host.Option{Inner: host.U32}}},
		Results: []host.ResultDecl{{Type: host.U32}},
	}
	hostInst := benchHostComponent(b, e, ty,
		func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, _ []uint64) error {
			// stack[0] already holds the option disc (and serves as the u32 result).
			return nil
		})

	caller := benchLoad(b, e, variantCallerWAT)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", hostInst.Core().ExportedFunc("f")))
	runBenchLoop(b, callerInst)
}

func BenchmarkCompToComp_Variant(b *testing.B) {
	e := benchEngine(b)
	calleeInst := benchInstantiate(b, benchLoad(b, e, watVariantCallee))

	caller := benchLoad(b, e, variantCallerWAT)
	callerInst := benchInstantiate(b, caller,
		wacogo.WithFuncImport("f", calleeInst.ExportedFunc("f")))
	runBenchLoop(b, callerInst)
}

// itoa is a small allocation-free int→string used to label subtests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
