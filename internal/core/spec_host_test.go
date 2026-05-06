package core_test

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// dropCounters tracks the number of resource1 dtor invocations and
// the rep of the most recent drop, mirroring the m1 globals in the
// previous host.wat shim.
type dropCounters struct {
	count    uint32
	lastDrop uint32
}

// minimalSimpleModule encodes:
//
//	(module
//	  (func (export "f") (result i32) i32.const 101)
//	  (global (export "g") i32 i32.const 100))
//
// Hand-encoded so the spec-host construction has no wat2wasm
// dependency.
var minimalSimpleModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f, // type 0: () -> (i32)
	0x03, 0x02, 0x01, 0x00, // funcs: [type 0]
	// global 0: i32, immutable, init = sleb128(100) = 0xE4 0x00 (bit 6 of
	// 0x64 is set, so the single-byte form would decode as -28).
	0x06, 0x07, 0x01, 0x7f, 0x00, 0x41, 0xe4, 0x00, 0x0b,
	0x07, 0x09, 0x02,
	0x01, 'f', 0x00, 0x00, // export "f" func 0
	0x01, 'g', 0x03, 0x00, // export "g" global 0
	0x0a, 0x07, 0x01, 0x05, 0x00, 0x41, 0xe5, 0x00, 0x0b, // code: i32.const 101 (sleb128) end
}

// topLevelExportNames lists the names that must additionally appear as
// top-level options. The wast suite imports some of these as bare
// top-level imports rather than via the "host" instance import.
// Resource type exports are skipped: the wast suite pulls them through
// the "host" instance import.
var topLevelExportNames = []string{
	"[constructor]resource1",
	"[static]resource1.assert",
	"[static]resource1.last-drop",
	"[static]resource1.drops",
	"[method]resource1.simple",
	"[method]resource1.take-borrow",
	"[method]resource1.take-own",
	"return-two",
	"return-three",
	"return-four",
	"host-return-two",
	"nested",
	"simple-module",
}

// buildSpecHost constructs the host component used by the wasm-tools
// component-model wast suite via host.Builder, then returns a slice of
// core.InstantiateOption values that satisfy every import the suite may
// declare. Callers spread these on every Instantiate call; options
// whose names aren't imported are ignored.
func buildSpecHost(ctx context.Context, e *core.Engine) ([]core.InstantiateOption, error) {
	b := host.NewBuilder(e, "spec-host")

	// resource1: own counted drops; resource2: distinct identity;
	// resource1-again: alias of resource1.
	//
	// Canon-rep == ExternHandle: the constructor stores the
	// user-supplied rep value as the extern object keyed by ExternHandle
	// and uses that ExternHandle as the canonical-ABI rep. The dtor
	// receives the obj it stored (the user-supplied rep). assert and
	// method.simple dereference canon-rep → ExternHandle → user-rep.
	resource1 := b.AddResource("resource1", func(_ context.Context, h *host.ComponentInstance, obj any) error {
		dc := h.UserState().(*dropCounters)
		dc.count++
		if rep, ok := obj.(uint32); ok {
			dc.lastDrop = rep
		}
		return nil
	})
	_ = b.AddResource("resource2", nil)
	if err := b.AddResourceAlias("resource1-again", resource1); err != nil {
		return nil, fmt.Errorf("alias: %w", err)
	}

	// [constructor]resource1(r u32) -> own<resource1>: register r on
	// the extern table and use the returned ExternHandle as the
	// canon-rep, so cross-component resource transfers preserve a
	// stable canon-rep that maps back to the user-supplied r.
	b.AddFunction("[constructor]resource1",
		&host.FuncType{
			Params:  []host.Param{{Name: "r", Type: host.U32}},
			Results: []host.ResultDecl{{Name: "", Type: resource1.Own()}},
		},
		func(_ context.Context, cc *core.CallContext, h *host.ComponentInstance, stack []uint64) error {
			r := uint32(stack[0])
			tr, ok := cc.Instance().ExportedType("resource1").(*core.TypeResource)
			if !ok {
				return fmt.Errorf("[constructor]resource1: resource1 not a *TypeResource")
			}
			eh := h.RegisterResource(r)
			handle := cc.IssueOwnHandle(tr, uint32(eh))
			stack[0] = uint64(handle.HandleID())
			return nil
		})

	// [static]resource1.assert(r own<resource1>, rep u32): the lower
	// step transfers the own into the host canon table; the handle
	// argument is a real index there. Look up its canon-rep
	// (ExternHandle), resolve to the user-rep via the extern table,
	// and compare.
	b.AddFunction("[static]resource1.assert",
		&host.FuncType{
			Params: []host.Param{
				{Name: "r", Type: resource1.Own()},
				{Name: "rep", Type: host.U32},
			},
		},
		func(_ context.Context, cc *core.CallContext, h *host.ComponentInstance, stack []uint64) error {
			handle := uint32(stack[0])
			expected := uint32(stack[1])
			tr, ok := cc.Instance().ExportedType("resource1").(*core.TypeResource)
			if !ok {
				return fmt.Errorf("[static]resource1.assert: resource1 not a *TypeResource")
			}
			rh, err := cc.LookupOwn(tr, handle)
			if err != nil {
				return fmt.Errorf("[static]resource1.assert: lookup own handle %d: %w", handle, err)
			}
			obj, ok := h.LookupResource(host.ExternHandle(rh.Rep()))
			if !ok {
				return fmt.Errorf("[static]resource1.assert: extern lookup failed for canon-rep %d", rh.Rep())
			}
			userRep := obj.(uint32)
			if userRep != expected {
				return fmt.Errorf("[static]resource1.assert: rep mismatch got=%d want=%d", userRep, expected)
			}
			return nil
		})

	// [static]resource1.last-drop / drops: return user-state counters.
	b.AddFunction("[static]resource1.last-drop",
		&host.FuncType{
			Results: []host.ResultDecl{{Name: "", Type: host.U32}},
		},
		func(_ context.Context, _ *core.CallContext, h *host.ComponentInstance, stack []uint64) error {
			dc := h.UserState().(*dropCounters)
			stack[0] = uint64(dc.lastDrop)
			return nil
		})
	b.AddFunction("[static]resource1.drops",
		&host.FuncType{
			Results: []host.ResultDecl{{Name: "", Type: host.U32}},
		},
		func(_ context.Context, _ *core.CallContext, h *host.ComponentInstance, stack []uint64) error {
			dc := h.UserState().(*dropCounters)
			stack[0] = uint64(dc.count)
			return nil
		})

	// [method]resource1.simple(self borrow<resource1>, rep u32): the
	// canon same-component shortcut for borrows of a resource whose
	// defining instance is the callee passes the canon-rep verbatim
	// (no callee-side borrow entry), so stack[0] is the canon-rep
	// (ExternHandle). Resolve it to the user-rep via the extern
	// table.
	b.AddFunction("[method]resource1.simple",
		&host.FuncType{
			Params: []host.Param{
				{Name: "self", Type: resource1.Borrow()},
				{Name: "rep", Type: host.U32},
			},
		},
		func(_ context.Context, _ *core.CallContext, h *host.ComponentInstance, stack []uint64) error {
			canonRep := uint32(stack[0])
			expected := uint32(stack[1])
			obj, ok := h.LookupResource(host.ExternHandle(canonRep))
			if !ok {
				return fmt.Errorf("[method]resource1.simple: extern lookup failed for canon-rep %d", canonRep)
			}
			userRep := obj.(uint32)
			if userRep != expected {
				return fmt.Errorf("[method]resource1.simple: rep mismatch got=%d want=%d", userRep, expected)
			}
			return nil
		})

	// [method]resource1.take-borrow(self borrow, b borrow): noop.
	b.AddFunction("[method]resource1.take-borrow",
		&host.FuncType{
			Params: []host.Param{
				{Name: "self", Type: resource1.Borrow()},
				{Name: "b", Type: resource1.Borrow()},
			},
		},
		func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
			return nil
		})

	// [method]resource1.take-own(self borrow, b own): noop. The own b
	// has been transferred into the host's table; we drop it on the
	// floor (the dtor will fire when no further reference is held —
	// actually canon will drop it via resource.drop on the caller side
	// before transferring out, but here it has already been transferred
	// in, so we own it). Drop explicitly so the dtor fires.
	b.AddFunction("[method]resource1.take-own",
		&host.FuncType{
			Params: []host.Param{
				{Name: "self", Type: resource1.Borrow()},
				{Name: "b", Type: resource1.Own()},
			},
		},
		func(_ context.Context, cc *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
			handle := uint32(stack[1])
			tr, ok := cc.Instance().ExportedType("resource1").(*core.TypeResource)
			if !ok {
				return fmt.Errorf("[method]resource1.take-own: resource1 not a *TypeResource")
			}
			rh, err := cc.LookupOwn(tr, handle)
			if err != nil {
				return fmt.Errorf("[method]resource1.take-own: lookup own %d: %w", handle, err)
			}
			return rh.Drop(ctx)
		})

	// Plain helpers.
	b.AddFunction("return-two", returnConstFnType(), returnConstFn(2))
	b.AddFunction("return-three", returnConstFnType(), returnConstFn(3))
	b.AddFunction("return-four", returnConstFnType(), returnConstFn(4))
	b.AddFunction("host-return-two", returnConstFnType(), returnConstFn(2))

	// Nested instance: re-exports return-four.
	nested := b.AddNestedInstance("nested")
	nested.AddFunction("return-four", returnConstFnType(), returnConstFn(4))

	// Core module: simple-module.
	if err := b.AddCoreModule("simple-module", minimalSimpleModule); err != nil {
		return nil, fmt.Errorf("simple-module: %w", err)
	}

	comp, err := b.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	hi, err := comp.Instantiate(ctx, host.WithUserState(&dropCounters{}))
	if err != nil {
		return nil, fmt.Errorf("instantiate: %w", err)
	}

	hostInst := hi.Core()
	opts := []core.InstantiateOption{core.WithInstanceImport("host", hostInst)}
	for _, name := range topLevelExportNames {
		if fn := hostInst.ExportedFunc(name); fn != nil {
			opts = append(opts, core.WithFuncImport(name, fn))
			continue
		}
		if sub := hostInst.ExportedInstance(name); sub != nil {
			opts = append(opts, core.WithInstanceImport(name, sub))
			continue
		}
		if cm := hostInst.ExportedModule(name); cm != nil {
			opts = append(opts, core.WithModuleImport(name, cm))
			continue
		}
	}
	return opts, nil
}

func returnConstFnType() *host.FuncType {
	return &host.FuncType{
		Results: []host.ResultDecl{{Name: "", Type: host.U32}},
	}
}

func returnConstFn(v uint32) host.Func {
	return func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		stack[0] = uint64(v)
		return nil
	}
}
