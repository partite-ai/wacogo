package host

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// Component is a reusable host-component template returned by
// Builder.Build. Each Instantiate call produces an independent
// ComponentInstance.
//
// A Component is safe for concurrent Instantiate calls.
type Component struct {
	engine       *core.Engine
	name         string
	arena        *wasmparser.TypeArena
	compiledStub wazero.CompiledModule

	// wpComponentType is the frozen wasmparser ComponentType handle
	// describing this Component's exports, with each ResourceTypeRef as a
	// placeholder. Built once by Builder.Build; each Instantiate binds those
	// placeholders with NewInstance.
	wpComponentType *wasmparser.ComponentType
	// root is the *scopeRuntime tree mirroring the build-time scope tree.
	root *scopeRuntime

	// allFuncs / allResources are flat lists across the whole tree (in
	// build order). Used by compileStubModule and buildPerInstanceHostMod
	// which produce one stub module + one host module spanning the tree.
	allFuncs     []*funcRuntime
	allResources []*resourceRuntime

	// resourceRefs holds the ResourceTypeRef declarations of every scope.
	// resolveResourceRefs reads from here.
	resourceRefs []resourceRefDecl

	closed atomic.Bool
}

// scopeRuntime is the per-scope runtime view paralleling *scope.
type scopeRuntime struct {
	exportName   string // "" at the root
	funcs        []*funcRuntime
	types        []typeRuntime
	resources    []*resourceRuntime
	resourceRefs []resourceRefDecl
	aliases      []aliasRuntime
	coreModules  []coreModuleRuntime
	nested       []*scopeRuntime
}

// typeRuntime is a named (exported) defined-type declaration. AddType
// calls with an empty name share a type internally without exporting
// it and never reach here.
type typeRuntime struct {
	exportName string
	ref        *TypeRef
}

type aliasRuntime struct {
	exportName string
	target     *resourceRuntime
}

type coreModuleRuntime struct {
	exportName string
	compiled   *core.CompiledModule
	typeDesc   wasmparser.CoreModuleTypeDesc
}

type funcRuntime struct {
	exportName     string // component-level export name (user-visible)
	stubExportName string // synthetic, unique within the stub: "__fn_<flatIdx>"
	userFn         Func
	userFT         *FuncType
	hostModExport  string // import name on the stub (matches stubExportName for funcs)
	flatParams     []byte
	flatResults    []byte
}

type resourceRuntime struct {
	exportName    string
	userDtor      ResourceDtor
	hostModExport string
	rt            *ResourceType
}

// Instantiate creates a fresh instance of this host component. The
// returned ComponentInstance has independent resource state and
// resource-type identity from any other instance of the same
// Component.
//
// Pass options to attach per-instance state (WithUserState) or
// resolve resource-type references (WithResourceFrom).
func (c *Component) Instantiate(ctx context.Context, opts ...InstantiateOption) (*ComponentInstance, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("wacogo/host: Instantiate after Close")
	}

	var iopts instantiateOpts
	for _, o := range opts {
		o.apply(&iopts)
	}

	refResolved, err := c.resolveResourceRefs(iopts.resourceFroms)
	if err != nil {
		return nil, err
	}
	bindings, err := buildResourceArgs(c, iopts.resourceFroms)
	if err != nil {
		return nil, err
	}

	// 1. Allocate the wrapper shell up front. Per-instance host-module
	//    closures and per-resource dtor closures capture h directly; no
	//    closure fires before construction completes.
	h := &ComponentInstance{
		extTable:     &externTable{},
		userState:    iopts.userState,
		callListener: iopts.callListener,
	}

	// 2. Per-instance host module + stub instantiation; closures capture h.
	hostMod, hostCode, err := buildPerInstanceHostMod(ctx, core.WazeroRuntime(c.engine), c, h)
	if err != nil {
		return nil, err
	}
	h.hostCode = hostCode
	stubMod, err := instantiateStubModule(ctx, core.WazeroRuntime(c.engine), c.compiledStub, hostMod)
	if err != nil {
		_ = hostMod.Close(ctx)
		_ = hostCode.Close(ctx)
		return nil, err
	}

	// 3. Resolve the stub's memory + realloc once for the canon.Callee
	//    construction below. The reusable host-call CallContext (see
	//    step after h.core is set) also closes over these.
	stubMemory := stubMod.Memory()
	reallocAPI := stubMod.ExportedFunction("realloc")
	alloc, err := newStubAllocator(stubMod)
	if err != nil {
		_ = stubMod.Close(ctx)
		_ = hostMod.Close(ctx)
		_ = hostCode.Close(ctx)
		return nil, err
	}

	// 4. Compute this instance's type, in which each ResourceTypeRef is the
	//    lender's resource, so guests' import type checks see that identity.
	wpInst := c.wpComponentType.NewInstance(bindings)

	// 5. Build the core instance. Its contents refer back to it, so they
	//    are assembled in the BuildContents callback.
	coreInst, err := core.NewInstance(c.engine, &core.InstanceSpec{
		Modules:            []api.Module{stubMod, hostMod},
		ParserInstanceType: wpInst,
		ExternTable:        h.extTable,
		BuildExports: func(inst *core.ComponentInstance) ([]core.InstanceExport, error) {
			// Mint the resource types inst defines. Every resource in
			// the tree is defined here whichever scope declares it:
			// this instance's canon table holds the handles, its
			// extern table backs the dtors, and its reentrance gate
			// wraps them. Nested scopes are namespaces over those
			// definitions, not full components of their own.
			res, err := c.newInstanceResources(inst, stubMod, h, refResolved)
			if err != nil {
				return nil, err
			}
			// Resolve the named AddType declarations across the tree.
			namedTypes, err := c.buildNamedTypes(res)
			if err != nil {
				return nil, err
			}
			// Exported funcs, flat across the tree and indexed parallel
			// to c.allFuncs, which is the type's func index space.
			funcExports, err := c.buildFuncExports(inst, wpInst, h, stubMod, stubMemory, reallocAPI, alloc, res.byComponentResourceTypeID)
			if err != nil {
				return nil, err
			}
			return c.buildScopeExports(c.root, wpInst, res, namedTypes, funcExports)
		},
	})
	if err != nil {
		_ = stubMod.Close(ctx)
		_ = hostMod.Close(ctx)
		_ = hostCode.Close(ctx)
		return nil, err
	}

	h.core = coreInst
	h.hostCallCC = core.NewCallContext(h.core, nil, stubMemory, alloc.realloc)

	return h, nil
}

// buildFuncExports produces one *core.ExportedFunc per entry in
// c.allFuncs, in the same order, each bound to a canon callee that
// enters inst.
func (c *Component) buildFuncExports(
	inst *core.ComponentInstance,
	wp *wasmparser.InstanceType,
	h *ComponentInstance,
	stubMod api.Module,
	stubMemory api.Memory,
	reallocAPI api.Function,
	alloc *stubAllocator,
	trSlice []*core.TypeResource,
) ([]*core.ExportedFunc, error) {
	funcExports := make([]*core.ExportedFunc, len(c.allFuncs))
	for i, fr := range c.allFuncs {
		coreFn := stubMod.ExportedFunction(fr.stubExportName)
		if coreFn == nil {
			return nil, fmt.Errorf("wacogo/host: stub missing export %q", fr.stubExportName)
		}
		coreFT, err := buildCoreFuncType(fr.userFT, trSlice)
		if err != nil {
			return nil, fmt.Errorf("wacogo/host: func %q: %w", fr.exportName, err)
		}
		callee := canon.Callee{
			CallSide: canon.CallSide{
				Instance:       core.InstanceAsCanon(inst),
				Memory:         stubMemory,
				Realloc:        reallocAPI,
				GoRealloc:      alloc.realloc,
				StringEncoding: canon.EncUTF8,
			},
			CoreFunc: coreFn,
			// What coreFn does - the stub wrapper resetting the bump
			// allocator, then the host module's Go function - without
			// going through wasm.
			Direct: func(ctx context.Context, stack []uint64) {
				alloc.reset()
				instrumentCall(ctx, h, CallKindFunction, fr.exportName, stack, true, func() error {
					return fr.userFn(ctx, h.hostCallCC, h, stack)
				})
			},
		}
		// Results of a host function are produced by Go code, not a
		// guest, so their lift is not bounded.
		binding := canon.NewCallBinding(
			core.FuncTypeParamsAsCanon(coreFT),
			core.FuncTypeResultsAsCanon(coreFT),
			callee,
			0,
		)
		funcExports[i] = core.NewExportedFunc(
			fr.exportName,
			core.NewFunc(coreFT, binding),
			wp.FuncType(i),
		)
	}
	return funcExports, nil
}

// buildScopeExports emits the InstanceExport list for one scope,
// recursing for nested instances. A nested instance is a bare shell —
// no modules, no extern table — that names the types its enclosing
// instance defines. It defines nothing itself, so its type shares wp's
// resource identities.
func (c *Component) buildScopeExports(
	sr *scopeRuntime,
	wp *wasmparser.InstanceType,
	res *instanceResources,
	namedTypes map[*TypeRef]core.Type,
	funcExports []*core.ExportedFunc,
) ([]core.InstanceExport, error) {
	funcIdx := map[*funcRuntime]int{}
	for i, fr := range c.allFuncs {
		funcIdx[fr] = i
	}
	out := make([]core.InstanceExport, 0,
		len(sr.funcs)+len(sr.types)+len(sr.resources)+len(sr.resourceRefs)+
			len(sr.aliases)+len(sr.coreModules)+len(sr.nested))
	for _, fr := range sr.funcs {
		out = append(out, core.InstanceExport{
			Name:    fr.exportName,
			Kind:    core.InstanceKindFunc,
			FuncVal: funcExports[funcIdx[fr]],
		})
	}
	for _, tr := range sr.types {
		out = append(out, core.InstanceExport{
			Name: tr.exportName, Kind: core.InstanceKindType,
			Type: namedTypes[tr.ref],
		})
	}
	for _, rr := range sr.resources {
		out = append(out, core.InstanceExport{
			Name: rr.exportName, Kind: core.InstanceKindType,
			Type: res.trOf[rr],
		})
	}
	for _, rrd := range sr.resourceRefs {
		out = append(out, core.InstanceExport{
			Name: rrd.name, Kind: core.InstanceKindType,
			Type: res.byComponentResourceTypeID[rrd.ref.componentResourceTypeID],
		})
	}
	for _, ar := range sr.aliases {
		out = append(out, core.InstanceExport{
			Name: ar.exportName, Kind: core.InstanceKindType,
			Type: res.trOf[ar.target],
		})
	}
	for _, cm := range sr.coreModules {
		out = append(out, core.InstanceExport{
			Name:           cm.exportName,
			Kind:           core.InstanceKindCoreModule,
			CompiledModule: cm.compiled,
		})
	}
	for _, child := range sr.nested {
		childWp := wp.ExportedInstance(child.exportName)
		childExports, err := c.buildScopeExports(child, childWp, res, namedTypes, funcExports)
		if err != nil {
			return nil, err
		}
		childInst, err := core.NewInstance(c.engine, &core.InstanceSpec{
			ParserInstanceType: childWp,
			BuildExports: func(*core.ComponentInstance) ([]core.InstanceExport, error) {
				return childExports, nil
			},
		})
		if err != nil {
			return nil, fmt.Errorf("wacogo/host: nested instance %q: %w", child.exportName, err)
		}
		out = append(out, core.InstanceExport{
			Name:     child.exportName,
			Kind:     core.InstanceKindInstance,
			Instance: childInst,
		})
	}
	return out, nil
}

// instanceResources is one Instantiate call's resource wiring: the
// fresh TypeResources the instance defines, plus the lookups that map
// build-time declarations onto them.
type instanceResources struct {
	// trOf gives the TypeResource minted for each declared resource.
	trOf map[*resourceRuntime]*core.TypeResource
	// byComponentResourceTypeID indexes defined resources and lender-
	// supplied ResourceTypeRefs in one shared ID space.
	byComponentResourceTypeID []*core.TypeResource
}

// newInstanceResources mints the per-instance TypeResources defined by
// inst. Every resource in the tree is defined by the root instance
// whichever scope declares it — a nested scope is a namespace over
// those definitions, not a runtime of its own.
func (c *Component) newInstanceResources(
	inst *core.ComponentInstance,
	stubMod api.Module,
	h *ComponentInstance,
	refResolved map[*ResourceTypeRef]*core.TypeResource,
) (*instanceResources, error) {
	res := &instanceResources{
		trOf:                      make(map[*resourceRuntime]*core.TypeResource, len(c.allResources)),
		byComponentResourceTypeID: make([]*core.TypeResource, len(c.allResources)+len(c.resourceRefs)),
	}
	for _, rr := range c.allResources {
		if rr.hostModExport != "" && stubMod.ExportedFunction(rr.hostModExport) == nil {
			return nil, fmt.Errorf("wacogo/host: stub missing dtor export %q", rr.hostModExport)
		}
		rrCapture := rr // by-pointer; userDtor is stable
		dtor := func(ctx context.Context, rep uint32) error {
			obj, _ := h.releaseResource(ExternHandle(rep))
			// obj may be nil when the rep wasn't registered through
			// RegisterResource (e.g., the guest minted the handle via
			// canon resource.new with a rep it chose). User dtors that
			// rely on the extern object should guard against nil.
			if rrCapture.userDtor != nil {
				return rrCapture.userDtor(ctx, h, obj)
			}
			return nil
		}
		tr := core.NewTypeResource(inst, dtor)
		res.trOf[rr] = tr
		res.byComponentResourceTypeID[rr.rt.componentResourceTypeID] = tr
	}
	for _, rrd := range c.resourceRefs {
		tr, ok := refResolved[rrd.ref]
		if !ok {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q unresolved", rrd.name)
		}
		res.byComponentResourceTypeID[rrd.ref.componentResourceTypeID] = tr
	}
	return res, nil
}

// buildNamedTypes resolves every named AddType declaration in the
// scope tree against this instance's resource identities, keyed by the
// declaration's *TypeRef so per-scope export lists can name it.
func (c *Component) buildNamedTypes(res *instanceResources) (map[*TypeRef]core.Type, error) {
	out := map[*TypeRef]core.Type{}
	var collect func(sr *scopeRuntime) error
	collect = func(sr *scopeRuntime) error {
		for _, tr := range sr.types {
			t, err := buildCoreType(tr.ref, res.byComponentResourceTypeID)
			if err != nil {
				return fmt.Errorf("wacogo/host: AddType %q: %w", tr.exportName, err)
			}
			out[tr.ref] = t
		}
		for _, child := range sr.nested {
			if err := collect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(c.root); err != nil {
		return nil, err
	}
	return out, nil
}

// resolveResourceRefs walks the Component's resourceRefs in
// declaration order, matches each against the supplied
// resourceFromOpts, and returns a map[*ResourceTypeRef]*core.TypeResource
// suitable for buildCoreFuncType. Errors if any ref is missing a
// pairing or if the lender doesn't export the named resource.
func (c *Component) resolveResourceRefs(opts []resourceFromOpt) (map[*ResourceTypeRef]*core.TypeResource, error) {
	resolved := make(map[*ResourceTypeRef]*core.TypeResource, len(c.resourceRefs))

	optByRef := make(map[*ResourceTypeRef]resourceFromOpt, len(opts))
	for _, o := range opts {
		if o.ref == nil {
			return nil, fmt.Errorf("wacogo/host: WithResourceFrom: nil ref")
		}
		if o.lender == nil {
			return nil, fmt.Errorf("wacogo/host: WithResourceFrom: nil lender for ref %q", o.ref.exportName)
		}
		optByRef[o.ref] = o
	}

	for _, rr := range c.resourceRefs {
		opt, ok := optByRef[rr.ref]
		if !ok {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q has no WithResourceFrom binding", rr.name)
		}
		tr, err := lookupExportedTypeResource(opt.lender, opt.lenderExport)
		if err != nil {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q: %w", rr.name, err)
		}
		resolved[rr.ref] = tr
	}
	return resolved, nil
}

// buildComponentWpType pushes every wasmparser type the host Component
// advertises into its arena, using each ResourceTypeRef's
// placeholderRID for refs. Returns a frozen *ComponentType handle, whose func
// index space is parallel to c.allFuncs. Called once from Builder.Build; never
// reads per-Instantiate state.
func buildComponentWpType(c *Component) (*wasmparser.ComponentType, error) {
	htb := c.arena.HostTypeBuilder()
	xt := newInstTranslator(htb)

	for _, rr := range c.allResources {
		xt.resResID[rr.rt] = htb.AllocResourceID()
	}
	for _, rrd := range c.resourceRefs {
		xt.refResID[rrd.ref] = rrd.placeholderRID
	}

	// Translate every func signature in build order so the func index space
	// is parallel to c.allFuncs.
	funcIDs := make([]wasmparser.ComponentFuncTypeID, len(c.allFuncs))
	funcIdx := map[*funcRuntime]int{}
	for i, fr := range c.allFuncs {
		fd, err := xt.instTranslateFuncType(fr.userFT)
		if err != nil {
			return nil, fmt.Errorf("wacogo/host: func %q: %w", fr.exportName, err)
		}
		id := htb.PushFuncType(fd)
		funcIDs[i] = id
		funcIdx[fr] = i
	}

	// Recursively build the wasmparser exports map for one scope. For
	// nested scopes this pushes a sub-ComponentTypeDesc + sub-
	// InstanceTypeDesc into the arena and references it via
	// EntityInstance{InstID:...} on the parent.
	var buildScopeExports func(sr *scopeRuntime) (map[string]wasmparser.ComponentEntityType, []string, error)
	buildScopeExports = func(sr *scopeRuntime) (map[string]wasmparser.ComponentEntityType, []string, error) {
		exp := make(map[string]wasmparser.ComponentEntityType,
			len(sr.funcs)+len(sr.types)+len(sr.resources)+len(sr.resourceRefs)+len(sr.aliases)+len(sr.coreModules)+len(sr.nested))
		var order []string
		for _, fr := range sr.funcs {
			exp[fr.exportName] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityFunc, FuncID: funcIDs[funcIdx[fr]],
			}
			order = append(order, fr.exportName)
		}
		for _, tr := range sr.types {
			id, err := xt.instTranslateDefinedType(tr.ref)
			if err != nil {
				return nil, nil, fmt.Errorf("wacogo/host: AddType %q: %w", tr.exportName, err)
			}
			exp[tr.exportName] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityType,
				TypeRef: wasmparser.ComponentAnyTypeID{
					Kind:  wasmparser.AnyTypeDefined,
					Index: uint32(id),
				},
			}
			order = append(order, tr.exportName)
		}
		for _, rr := range sr.resources {
			exp[rr.exportName] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityType,
				TypeRef: wasmparser.ComponentAnyTypeID{
					Kind:  wasmparser.AnyTypeResource,
					ResID: xt.resResID[rr.rt],
				},
			}
			order = append(order, rr.exportName)
		}
		for _, rrd := range sr.resourceRefs {
			exp[rrd.name] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityType,
				TypeRef: wasmparser.ComponentAnyTypeID{
					Kind:  wasmparser.AnyTypeResource,
					ResID: xt.refResID[rrd.ref],
				},
			}
			order = append(order, rrd.name)
		}
		for _, ar := range sr.aliases {
			exp[ar.exportName] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityType,
				TypeRef: wasmparser.ComponentAnyTypeID{
					Kind:  wasmparser.AnyTypeResource,
					ResID: xt.resResID[ar.target.rt],
				},
			}
			order = append(order, ar.exportName)
		}
		for _, cmRT := range sr.coreModules {
			modID := htb.PushCoreModuleType(cmRT.typeDesc)
			exp[cmRT.exportName] = wasmparser.ComponentEntityType{
				Kind:     wasmparser.EntityModule,
				ModuleID: modID,
			}
			order = append(order, cmRT.exportName)
		}
		for _, child := range sr.nested {
			childExp, childOrder, err := buildScopeExports(child)
			if err != nil {
				return nil, nil, err
			}
			ctID := htb.PushComponentType(wasmparser.ComponentTypeDesc{
				Imports:     map[string]wasmparser.ComponentEntityType{},
				ImportOrder: nil,
				Exports:     childExp,
				ExportOrder: childOrder,
			})
			subInstID := htb.PushInstanceType(wasmparser.InstanceTypeDesc{
				Exports: childExp,
			})
			_ = ctID
			exp[child.exportName] = wasmparser.ComponentEntityType{
				Kind:   wasmparser.EntityInstance,
				InstID: subInstID,
			}
			order = append(order, child.exportName)
		}
		return exp, order, nil
	}

	exports, exportOrder, err := buildScopeExports(c.root)
	if err != nil {
		return nil, err
	}
	open := make([]wasmparser.ImportedResource, len(c.resourceRefs))
	for i, rrd := range c.resourceRefs {
		open[i] = wasmparser.ImportedResource{ID: rrd.placeholderRID}
	}
	ctID := htb.PushComponentType(wasmparser.ComponentTypeDesc{
		Imports:           map[string]wasmparser.ComponentEntityType{},
		ImportOrder:       nil,
		Exports:           exports,
		ExportOrder:       exportOrder,
		Funcs:             funcIDs,
		ImportedResources: open,
	})
	return htb.NewComponentTypeHandle(ctID), nil
}

// resourceBindings answers what each ResourceTypeRef's placeholder stands for. A
// host component has no imports, so nothing else is ever asked of it.
type resourceBindings map[wasmparser.ResourceID]wasmparser.ResourceFrom

func (resourceBindings) Instance(string) *wasmparser.InstanceType { return nil }

func (b resourceBindings) Resource(res wasmparser.ImportedResource) (wasmparser.ResourceFrom, bool) {
	from, ok := b[res.ID]
	return from, ok
}

// buildResourceArgs binds each ResourceTypeRef's placeholder to the lender export
// that supplies it. Called once per Instantiate, after resolveResourceRefs has
// checked each ref's lender.
func buildResourceArgs(c *Component, opts []resourceFromOpt) (resourceBindings, error) {
	if len(c.resourceRefs) == 0 {
		return nil, nil
	}
	optByRef := make(map[*ResourceTypeRef]resourceFromOpt, len(opts))
	for _, o := range opts {
		optByRef[o.ref] = o
	}
	resources := make(resourceBindings, len(c.resourceRefs))
	for _, rrd := range c.resourceRefs {
		lenderWp := optByRef[rrd.ref].lender.ParserInstanceType()
		if lenderWp == nil {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q: lender has no wasmparser type info", rrd.name)
		}
		resources[rrd.placeholderRID] = wasmparser.ResourceFrom{Lender: lenderWp, Export: optByRef[rrd.ref].lenderExport}
	}
	return resources, nil
}

func lookupExportedTypeResource(lender *core.ComponentInstance, exportName string) (*core.TypeResource, error) {
	t := lender.ExportedType(exportName)
	if t == nil {
		return nil, fmt.Errorf("lender does not export type %q", exportName)
	}
	tr, ok := t.(*core.TypeResource)
	if !ok {
		return nil, fmt.Errorf("lender export %q is %T, want *TypeResource", exportName, t)
	}
	return tr, nil
}

// Close releases resources shared by every instance of this
// Component. After Close, Instantiate returns an error.
// Already-created instances remain usable until each is itself closed.
//
// Close is idempotent.
func (c *Component) Close(ctx context.Context) error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	if c.compiledStub != nil {
		return c.compiledStub.Close(ctx)
	}
	return nil
}
