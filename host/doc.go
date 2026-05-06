// Package host builds component-model component instances in Go.
//
// A host component declares one or more exported functions, types,
// and resource types in Go and can then be passed as an instance
// import to any wasm component loaded by the same engine. The wasm
// consumer sees a fully type-checked component instance
// indistinguishable from one loaded from a .wasm file.
//
// # Basic shape
//
//	b := engine.NewHostBuilder("my-host")
//	b.AddFunction("greet", &host.FuncType{
//	    Params:  []host.Param{{"name", host.String}},
//	    Results: []host.ResultDecl{{"", host.String}},
//	}, greetImpl)
//
//	comp, err := b.Build(ctx)
//	inst, err := comp.Instantiate(ctx)
//	// Pass inst.Core() to wacogo.WithInstanceImport when wiring
//	// this host component as an import.
//
// # Flat-boundary functions
//
// Host functions run at the canonical-ABI flat boundary: arguments
// and results pass through a []uint64 stack, with caller memory and
// resource-table operations accessed through the CallContext argument.
// Use a code generator (e.g. witgen) to produce typed wrappers around
// this raw shape.
//
// # Template and instance
//
// Builder.Build produces a Component template; each
// Component.Instantiate call yields an independent ComponentInstance
// with its own resource state. Close a ComponentInstance when the
// importing component is closed; close the Component when no more
// instances will be created.
package host
