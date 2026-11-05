package core

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestHostFuncs_Dispatch checks calls reach the right function when many in one
// module share a signature, across components that each have their own module.
func TestHostFuncs_Dispatch(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	provider, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (core module $m
    (func (export "one") (result i32) i32.const 1)
    (func (export "two") (result i32) i32.const 2)
    (func (export "three") (result i32) i32.const 3))
  (core instance $i (instantiate $m))
  (func (export "one") (result u32) (canon lift (core func $i "one")))
  (func (export "two") (result u32) (canon lift (core func $i "two")))
  (func (export "three") (result u32) (canon lift (core func $i "three"))))`)))
	if err != nil {
		t.Fatal(err)
	}
	pi, err := provider.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (import "one" (func $one (result u32)))
  (import "two" (func $two (result u32)))
  (import "three" (func $three (result u32)))
  (core func $l1 (canon lower (func $one)))
  (core func $l2 (canon lower (func $two)))
  (core func $l3 (canon lower (func $three)))
  (core instance $env
    (export "one" (func $l1)) (export "two" (func $l2)) (export "three" (func $l3)))
  (core module $m
    (import "env" "one" (func $one (result i32)))
    (import "env" "two" (func $two (result i32)))
    (import "env" "three" (func $three (result i32)))
    (func (export "go") (result i32)
      (i32.add (i32.mul (call $one) (i32.const 100))
        (i32.add (i32.mul (call $two) (i32.const 10)) (call $three)))))
  (core instance $i (instantiate $m (with "env" (instance $env))))
  (func (export "go") (result u32) (canon lift (core func $i "go"))))`)))
	if err != nil {
		t.Fatal(err)
	}
	// Two instances, so a slot of one can't be serving the other.
	for range 2 {
		inst, err := consumer.Instantiate(ctx,
			WithFuncImport("one", pi.ExportedFunc("one")),
			WithFuncImport("two", pi.ExportedFunc("two")),
			WithFuncImport("three", pi.ExportedFunc("three")))
		if err != nil {
			t.Fatal(err)
		}
		got, err := inst.ExportedFunc("go").Call(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != ValU32(123) {
			t.Fatalf("got %v, want 123", got)
		}
	}
}

// TestHostFuncs_LiftedLowered covers a lowered function called from Go rather than
// by a guest module, as when it is lifted again.
func TestHostFuncs_LiftedLowered(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (component $p
    (core module $m (func (export "f") (result i32) i32.const 7))
    (core instance $i (instantiate $m))
    (func (export "f") (result u32) (canon lift (core func $i "f"))))
  (instance $pi (instantiate $p))
  (alias export $pi "f" (func $f))
  (core func $lowered (canon lower (func $f)))
  (func (export "again") (result u32) (canon lift (core func $lowered))))`)))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := inst.ExportedFunc("again").Call(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != ValU32(7) {
		t.Fatalf("got %v, want 7", got)
	}
}

// TestHostFuncs_SplitAcrossModules checks a component with more functions than
// wazero allows in a module, calling the one that lands in the second module.
func TestHostFuncs_SplitAcrossModules(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (type $r (resource (rep i32)))`+
		strings.Repeat(` (core func (canon resource.rep $r))`, maxHostFuncsPerModule)+`
  (core func $last (canon resource.rep $r))
  (core instance $env (export "last" (func $last)))
  (core module $m
    (import "env" "last" (func $last (param i32) (result i32)))
    (func (export "go") (result i32) (call $last (i32.const 99))))
  (core instance $i (instantiate $m (with "env" (instance $env))))
  (func (export "go") (result u32) (canon lift (core func $i "go"))))`)))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Handle 99 doesn't exist, so reaching the right function means this trap.
	if _, err := inst.ExportedFunc("go").Call(ctx); err == nil || !strings.Contains(err.Error(), "resource.rep") {
		t.Fatalf("got %v", err)
	}
}

// TestHostFuncs_TraceNames traps through each kind of host function, checking a
// stack trace says what the component calls it rather than how it is linked.
func TestHostFuncs_TraceNames(t *testing.T) {
	const provider = `(component
  (core module $m (func (export "f") unreachable))
  (core instance $i (instantiate $m))
  (func (export "boom") (canon lift (core func $i "f"))))`
	const callLowered = `
  (core instance $env (export "f" (func $lowered)))
  (core module $m (import "env" "f" (func $f)) (func (export "go") call $f))
  (core instance $i (instantiate $m (with "env" (instance $env))))
  (func (export "go") (canon lift (core func $i "go"))))`

	for name, c := range map[string]struct {
		wat, want string
		instance  bool
	}{
		"imported func": {
			wat: `(component
  (import "boom" (func $f))
  (core func $lowered (canon lower (func $f)))` + callLowered,
			want: "wacogo_host_funcs.boom(",
		},
		"func from an imported instance": {
			wat: `(component
  (import "wasi:pkg/iface@1.0.0" (instance $inst (export "boom" (func))))
  (alias export $inst "boom" (func $f))
  (core func $lowered (canon lower (func $f)))` + callLowered,
			want:     "wacogo_host_funcs.wasi:pkg/iface@1.0.0#boom(",
			instance: true,
		},
		"func from an instance defined here": {
			wat: `(component
  (component $p ` + strings.TrimPrefix(provider, "(component") + `
  (instance $inst (instantiate $p))
  (alias export $inst "boom" (func $f))
  (core func $lowered (canon lower (func $f)))` + callLowered,
			want: "wacogo_host_funcs.boom(",
		},
		"resource builtin": {
			wat: `(component
  (type $r (resource (rep i32)))
  (core func $rep (canon resource.rep $r))
  (core instance $env (export "rep" (func $rep)))
  (core module $m
    (import "env" "rep" (func $rep (param i32) (result i32)))
    (func (export "go") (result i32) (call $rep (i32.const 99))))
  (core instance $i (instantiate $m (with "env" (instance $env))))
  (func (export "go") (result u32) (canon lift (core func $i "go"))))`,
			want: "wacogo_host_funcs.resource.rep(",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			engine := NewEngine(ctx)
			defer engine.Close(ctx)

			pc, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, provider)))
			if err != nil {
				t.Fatal(err)
			}
			pi, err := pc.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, c.wat)))
			if err != nil {
				t.Fatal(err)
			}
			var opts []InstantiateOption
			if c.instance {
				opts = append(opts, WithInstanceImport("wasi:pkg/iface@1.0.0", pi))
			} else if strings.Contains(c.wat, `(import "boom"`) {
				opts = append(opts, WithFuncImport("boom", pi.ExportedFunc("boom")))
			}
			inst, err := comp.Instantiate(ctx, opts...)
			if err != nil {
				t.Fatal(err)
			}
			_, err = inst.ExportedFunc("go").Call(ctx)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want a trace naming %q, got:\n%v", c.want, err)
			}
		})
	}
}
