package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo/wasmparser"
)

// TestInstanceLimits checks that the limits sit exactly at what an instantiation
// creates, and that the count of core instances is what the runtime creates.
func TestInstanceLimits(t *testing.T) {
	const leaf = `(component $c (core module $m) (core instance (instantiate $m)))`

	for name, c := range map[string]struct {
		wat             string
		core, component uint32
	}{
		"flat": {
			wat: `(component
  (core module $m)
  (core instance (instantiate $m))
  (core instance (instantiate $m)))`,
			core: 2, component: 1,
		},
		"inline core instance": {
			wat: `(component
  (core module $m (func (export "f")))
  (core instance $i (instantiate $m))
  (alias core export $i "f" (core func $f))
  (core instance (export "g" (func $f))))`,
			core: 2, component: 1,
		},
		"nested": {
			wat: `(component ` + leaf + `
  (instance (instantiate $c))
  (instance (instantiate $c)))`,
			core: 2, component: 3,
		},
		"multiplied by nesting": {
			wat: `(component
  (component $d ` + leaf + `
    (instance (instantiate $c))
    (instance (instantiate $c)))
  (instance (instantiate $d))
  (instance (instantiate $d)))`,
			core: 4, component: 7,
		},
		"component argument": {
			wat: `(component ` + leaf + `
  (component $u
    (import "x" (component $x))
    (instance (instantiate $x))
    (instance (instantiate $x)))
  (instance (instantiate $u (with "x" (component $c)))))`,
			core: 2, component: 4,
		},
		"outer alias": {
			wat: `(component ` + leaf + `
  (component $u
    (alias outer 1 0 (component $d))
    (instance (instantiate $d)))
  (instance (instantiate $u)))`,
			core: 1, component: 3,
		},
		"component from an instance": {
			wat: `(component ` + leaf + `
  (instance $i (export "c" (component $c)))
  (alias export $i "c" (component $d))
  (instance (instantiate $d)))`,
			core: 1, component: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			bin := parseLinkWat(t, c.wat)
			load := func(limits ParseLimits) error {
				engine := NewEngine(ctx, WithParseLimits(limits))
				defer engine.Close(ctx)
				_, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
				return err
			}

			if err := load(ParseLimits{CoreInstances: c.core, ComponentInstances: c.component}); err != nil {
				t.Fatalf("at the limits: %v", err)
			}
			if c.core > 1 {
				if err := load(ParseLimits{CoreInstances: c.core - 1}); !errors.Is(err, ErrInstanceLimitExceeded) {
					t.Fatalf("one core instance over: got %v", err)
				}
			}
			if c.component > 1 {
				if err := load(ParseLimits{ComponentInstances: c.component - 1}); !errors.Is(err, ErrInstanceLimitExceeded) {
					t.Fatalf("one component instance over: got %v", err)
				}
			}

			engine := NewEngine(ctx)
			defer engine.Close(ctx)
			comp, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
			if err != nil {
				t.Fatal(err)
			}
			before := instanceCounter.Load()
			if _, err := comp.Instantiate(ctx); err != nil {
				t.Fatal(err)
			}
			if created := instanceCounter.Load() - before; created != uint64(c.core) {
				t.Fatalf("runtime created %d core instances, counted %d", created, c.core)
			}
		})
	}
}

// TestInstanceLimits_Default checks both defaults, and that they can be raised.
func TestInstanceLimits_Default(t *testing.T) {
	core := `(component (core module $m)` + strings.Repeat(` (core instance (instantiate $m))`, 1001) + `)`
	components := `(component (component $c)` + strings.Repeat(` (instance (instantiate $c))`, 1000) + `)`

	for name, c := range map[string]struct {
		wat    string
		raised ParseLimits
	}{
		"core":      {core, ParseLimits{CoreInstances: 2000}},
		"component": {components, ParseLimits{ComponentInstances: 2000}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			bin := parseLinkWat(t, c.wat)

			engine := NewEngine(ctx)
			defer engine.Close(ctx)
			if _, err := engine.LoadComponent(ctx, bytes.NewReader(bin)); !errors.Is(err, ErrInstanceLimitExceeded) {
				t.Fatalf("default: got %v", err)
			}

			raised := NewEngine(ctx, WithParseLimits(c.raised))
			defer raised.Close(ctx)
			if _, err := raised.LoadComponent(ctx, bytes.NewReader(bin)); err != nil {
				t.Fatalf("raised: %v", err)
			}
		})
	}
}

// TestInstanceLimits_DeepNesting would not finish if the check visited every
// instance, as 40 levels of two instantiations each is 2^41 of them.
func TestInstanceLimits_DeepNesting(t *testing.T) {
	wat := `(component $c)`
	for range 40 {
		wat = `(component $c ` + wat + ` (instance (instantiate $c)) (instance (instantiate $c)))`
	}
	bin := parseLinkWat(t, wat)

	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)
	if _, err := engine.LoadComponent(ctx, bytes.NewReader(bin)); !errors.Is(err, ErrInstanceLimitExceeded) {
		t.Fatalf("got %v", err)
	}
}

// TestParseLimits checks that a limit reaches where it is enforced: each case
// loads by default and is rejected once the limit it exercises is lowered.
func TestParseLimits(t *testing.T) {
	twoParams := `(type (func (param "a" u32) (param "b" u32)))`

	for name, c := range map[string]struct {
		wat    string
		limits ParseLimits
	}{
		"component func params": {`(component ` + twoParams + `)`, ParseLimits{FunctionParams: 1}},
		"in a nested component": {`(component (component ` + twoParams + `))`, ParseLimits{FunctionParams: 1}},
		"in a deeper component": {`(component (component (component ` + twoParams + `)))`, ParseLimits{FunctionParams: 1}},
		"core func params":      {`(component (core module (type (func (param i32 i32)))))`, ParseLimits{FunctionParams: 1}},
		"core func results":     {`(component (core module (type (func (result i32 i32)))))`, ParseLimits{FunctionResults: 1}},
		"module type decl":      {`(component (core type (module (type (func)) (type (func)))))`, ParseLimits{ModuleTypeDeclarations: 1}},
		"functions":             {`(component (core module (func) (func)))`, ParseLimits{Functions: 1}},
		"locals":                {`(component (core module (func (local i32 i32))))`, ParseLimits{Functions: 1}},
		"imports":               {`(component (core module (import "a" "b" (func)) (import "a" "c" (func))))`, ParseLimits{Imports: 1}},
		"exports":               {`(component (core module (func $f) (export "a" (func $f)) (export "b" (func $f))))`, ParseLimits{Exports: 1}},
		"globals":               {`(component (core module (global i32 (i32.const 0)) (global i32 (i32.const 0))))`, ParseLimits{Globals: 1}},
		"tables":                {`(component (core module (table 0 funcref) (table 0 funcref)))`, ParseLimits{Tables: 1}},
		"types":                 {`(component (core module (type (func)) (type (func (param i32)))))`, ParseLimits{Types: 1}},
		"body size":             {`(component (core module (func nop nop nop nop)))`, ParseLimits{FunctionBodySize: 2}},
		"records":               {`(component (type (record (field "a" u32) (field "b" u32))))`, ParseLimits{RecordFields: 1}},
		"variants":              {`(component (type (variant (case "a") (case "b"))))`, ParseLimits{VariantCases: 1}},
		"tuples":                {`(component (type (tuple u32 u32)))`, ParseLimits{TupleTypes: 1}},
		"enums":                 {`(component (type (enum "a" "b")))`, ParseLimits{EnumCases: 1}},
		"strings":               {`(component (import "abc" (func)))`, ParseLimits{StringSize: 2}},
		"section size":          {`(component (type (tuple u32 u32 u32 u32)))`, ParseLimits{ModuleSize: 2}},
		"nesting":               {`(component (component (component)))`, ParseLimits{NestingDepth: 1}},
		"effective type size":   {`(component (type (tuple u32 u32)))`, ParseLimits{EffectiveTypeSize: 1}},
		"instance type decl":    {`(component (type (instance (type u32) (type u32))))`, ParseLimits{InstanceTypeDeclarations: 1}},
		"component type decl":   {`(component (type (component (type u32) (type u32))))`, ParseLimits{ComponentTypeDeclarations: 1}},
		"instantiation args": {`(component
  (import "x" (func $x)) (import "y" (func $y))
  (component $c (import "x" (func)) (import "y" (func)))
  (instance (instantiate $c (with "x" (func $x)) (with "y" (func $y)))))`, ParseLimits{InstantiationArgs: 1}},
		"extern names": {`(component
  (import "x" (func $x))
  (instance (export "a" (func $x)) (export "b" (func $x))))`, ParseLimits{ComponentExternNames: 1}},
		"canonical options": {`(component
  (core module $m (memory (export "m") 1) (func (export "f")))
  (core instance $i (instantiate $m))
  (core func $f (alias core export $i "f"))
  (func (canon lift (core func $f) (memory $i "m") string-encoding=utf8)))`, ParseLimits{CanonicalOptions: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			bin := parseLinkWat(t, c.wat)

			def := NewEngine(ctx)
			defer def.Close(ctx)
			if _, err := def.LoadComponent(ctx, bytes.NewReader(bin)); err != nil {
				t.Fatalf("default: %v", err)
			}

			lowered := NewEngine(ctx, WithParseLimits(c.limits))
			defer lowered.Close(ctx)
			if _, err := lowered.LoadComponent(ctx, bytes.NewReader(bin)); err == nil {
				t.Fatalf("not rejected under %+v", c.limits)
			}
		})
	}
}

// TestParseLimits_Raised checks that a raised limit holds through every read of
// a section, as validating and loading each read it. The binary is encoded by
// hand because wasm-tools won't encode a tuple past its own limit.
func TestParseLimits_Raised(t *testing.T) {
	const tupleSize = 10_001

	section := []byte{1, 0x6f} // one type: a tuple
	section = binary.AppendUvarint(section, tupleSize)
	section = append(section, bytes.Repeat([]byte{0x79}, tupleSize)...) // of u32

	bin := []byte{0x00, 'a', 's', 'm', 0x0d, 0x00, 0x01, 0x00}
	bin = append(bin, 7) // the component type section
	bin = binary.AppendUvarint(bin, uint64(len(section)))
	bin = append(bin, section...)

	ctx := context.Background()
	def := NewEngine(ctx)
	defer def.Close(ctx)
	if _, err := def.LoadComponent(ctx, bytes.NewReader(bin)); err == nil || !strings.Contains(err.Error(), "tuple") {
		t.Fatalf("default: got %v", err)
	}

	raised := NewEngine(ctx, WithParseLimits(ParseLimits{TupleTypes: 20_000}))
	defer raised.Close(ctx)
	if _, err := raised.LoadComponent(ctx, bytes.NewReader(bin)); err != nil {
		t.Fatalf("raised: %v", err)
	}
}

// TestParseLimits_Unlimited checks that Unlimited lifts a limit, for one that is
// compared with a count, the nesting depth, and the instance limits, whose check
// is skipped: this would not finish if it visited each of 2^41 instances.
func TestParseLimits_Unlimited(t *testing.T) {
	deep := `(component $c)`
	for range 40 {
		deep = `(component $c ` + deep + ` (instance (instantiate $c)) (instance (instantiate $c)))`
	}
	for name, c := range map[string]struct {
		wat    string
		limits ParseLimits
	}{
		"count":     {`(component (type (func (param "a" u32) (param "b" u32))))`, ParseLimits{FunctionParams: Unlimited}},
		"nesting":   {`(component (component (component)))`, ParseLimits{NestingDepth: Unlimited}},
		"instances": {deep, ParseLimits{CoreInstances: Unlimited, ComponentInstances: Unlimited}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			engine := NewEngine(ctx, WithParseLimits(c.limits))
			defer engine.Close(ctx)
			if _, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, c.wat))); err != nil {
				t.Fatal(err)
			}
		})
	}

	// One left at its default still applies.
	t.Run("other limits keep their defaults", func(t *testing.T) {
		ctx := context.Background()
		engine := NewEngine(ctx, WithParseLimits(ParseLimits{ComponentInstances: Unlimited}))
		defer engine.Close(ctx)
		bin := parseLinkWat(t, `(component (core module $m)`+strings.Repeat(` (core instance (instantiate $m))`, 1001)+`)`)
		if _, err := engine.LoadComponent(ctx, bytes.NewReader(bin)); !errors.Is(err, ErrInstanceLimitExceeded) {
			t.Fatalf("got %v", err)
		}
	})
}

// TestParseLimits_ValidatorOnly covers what an engine can't load: it never
// enables values, and wazero allows only one memory.
func TestParseLimits_ValidatorOnly(t *testing.T) {
	for name, c := range map[string]struct {
		wat    string
		limits ParseLimits
	}{
		"start args": {`(component
  (import "a" (value $a u32))
  (import "b" (value $b u32))
  (import "f" (func $f (param "x" u32) (param "y" u32)))
  (start $f (value $a) (value $b)))`, ParseLimits{StartArgs: 1}},
		"memories": {`(component (core module (memory 0) (memory 0)))`, ParseLimits{Memories: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			bin := parseLinkWat(t, c.wat)
			validate := func(opts ...wasmparser.ParseOption) error {
				vp := wasmparser.NewValidatingParser(bytes.NewReader(bin), wasmparser.AllFeatures(), opts...)
				for {
					if _, err := vp.Next(); err == io.EOF {
						return nil
					} else if err != nil {
						return err
					}
				}
			}
			if err := validate(); err != nil {
				t.Fatalf("default: %v", err)
			}
			if err := validate(wasmparser.WithParseLimits(c.limits)); err == nil {
				t.Fatal("not rejected")
			}
		})
	}
}
