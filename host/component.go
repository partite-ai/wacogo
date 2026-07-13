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
	// describing this Component's exports, with placeholder ResIDs
	// for ResourceTypeRefs. Built once by Builder.Build; per-Instantiate
	// resource identity is layered on via *InstanceType + SetResourceOrigin.
	wpComponentType *wasmparser.ComponentType
	// wpFuncTypes is parallel to allFuncs; each entry is a *FuncType
	// handle into c.arena. Read by ExportedFunc.ParserFunctionType.
	wpFuncTypes []*wasmparser.FuncType

	// root is the *scopeRuntime tree mirroring the build-time scope tree.
	root *scopeRuntime

	// allFuncs / allResources are flat lists across the whole tree (in
	// build order). Used by compileStubModule and buildPerInstanceHostMod
	// which produce one stub module + one host module spanning the tree.
	allFuncs     []*funcRuntime
	allResources []*resourceRuntime
	allTypes     []*typeRuntime

	// resourceRefs holds the root-scope ResourceTypeRef declarations.
	// resolveResourceRefs reads from here.
	resourceRefs []resourceRefDecl

	closed atomic.Bool
}

// scopeRuntime is the per-scope runtime view paralleling *scope.
type scopeRuntime struct {
	exportName   string // "" at the root
	funcs        []*funcRuntime
	types        []*typeRuntime
	resources    []*resourceRuntime
	resourceRefs []resourceRefDecl
	aliases      []aliasRuntime
	coreModules  []coreModuleRuntime
	nested       []*scopeRuntime
}

// typeRuntime retains one AddType declaration after Builder.Build. slot is
// stable across Component.Instantiate calls and indexes the AddType portion of
// its lexical scope's live type space; resource and resource-ref slots precede
// it.
type typeRuntime struct {
	exportName string
	ref        *TypeRef
	slot       uint32
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

	// 1. Allocate the wrapper shell up front. Per-instance host-module
	//    closures and per-resource dtor closures capture h directly; no
	//    closure fires before construction completes.
	h := &ComponentInstance{
		extTable:     &externTable{},
		userState:    iopts.userState,
		callListener: iopts.callListener,
	}

	// 2. Per-instance host module + stub instantiation; closures capture h.
	hostMod, err := buildPerInstanceHostMod(ctx, core.WazeroRuntime(c.engine), c, h)
	if err != nil {
		return nil, err
	}
	stubMod, err := instantiateStubModule(ctx, core.WazeroRuntime(c.engine), c.compiledStub, hostMod)
	if err != nil {
		_ = hostMod.Close(ctx)
		return nil, err
	}

	// 3. Resolve the stub's memory + realloc once for the canon.Callee
	//    construction below. The reusable host-call CallContext (see
	//    step after h.core is set) also closes over these.
	stubMemory := stubMod.Memory()
	reallocAPI := stubMod.ExportedFunction("realloc")
	stubRealloc := wrapRealloc(reallocAPI)

	// 4. Per-instance TypeResources, one per entry in comp.allResources.
	//    Per-resource dtor closures capture h directly — no UserData type
	//    assertion.
	resourceTRs := make([]*core.TypeResource, len(c.allResources))
	resourceTRIdx := make(map[*resourceRuntime]int, len(c.allResources))
	for i, rr := range c.allResources {
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
		resourceTRs[i] = core.NewTypeResource(dtor)
		resourceTRIdx[rr] = i
	}

	// 5. Build trSlice (resources + resourceRefs share one ID space).
	trSlice := make([]*core.TypeResource, len(c.allResources)+len(c.resourceRefs))
	for i, rr := range c.allResources {
		trSlice[rr.rt.componentResourceTypeID] = resourceTRs[i]
	}
	for _, rrd := range c.resourceRefs {
		tr, ok := refResolved[rrd.ref]
		if !ok {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q unresolved", rrd.name)
		}
		trSlice[rrd.ref.componentResourceTypeID] = tr
	}

	// 6. Build a per-instance wasmparser ComponentType reflecting each
	//    ResourceTypeRef's lender-supplied resource identity. The
	//    per-func wpFuncType handles minted here are the same identities
	//    advertised in the resulting *InstanceType, so guests' import
	//    type checks pass.
	origins, err := buildInstanceOrigins(c, iopts.resourceFroms)
	if err != nil {
		return nil, err
	}
	wpInst := c.wpComponentType.NewInstance()
	if len(origins) > 0 {
		wpInst.SetResourceOrigin(origins)
	}
	wpFuncTypes := c.wpFuncTypes

	// 7. Build the per-scope export lists. Funcs are pre-built once
	//    (flat across the whole tree, indexed parallel to c.allFuncs)
	//    inside the root's BuildExports callback so each canon.Callee
	//    captures the freshly-allocated root *ComponentInstance. Nested
	//    *ComponentInstance shells are built INSIDE the root callback
	//    too — they receive lexical AddType slot views, reference the same
	//    funcExports, and attach as InstanceKindInstance entries on the root.
	funcIdx := map[*funcRuntime]int{}
	for i, fr := range c.allFuncs {
		funcIdx[fr] = i
	}

	// Resource slots retain their existing root-instance layout. Each scope's
	// AddType slots follow that reserved prefix in stable local declaration
	// order. Nested live instances receive only their own AddType portion: this
	// makes named types addressable without changing nested-resource ownership.
	typeSlotBase := len(c.allResources) + len(c.resourceRefs)
	rootResourceSlots := make([]core.InstanceTypeSlot, 0, typeSlotBase)
	for i, rr := range c.allResources {
		rootResourceSlots = append(rootResourceSlots, core.InstanceTypeSlot{Name: rr.exportName, Type: resourceTRs[i]})
	}
	for _, rrd := range c.resourceRefs {
		tr, ok := refResolved[rrd.ref]
		if !ok {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q unresolved", rrd.name)
		}
		rootResourceSlots = append(rootResourceSlots, core.InstanceTypeSlot{Name: rrd.name, Type: tr})
	}
	typeValues := make(map[*typeRuntime]core.Type, len(c.allTypes))
	for _, tr := range c.allTypes {
		t, err := buildCoreType(tr.ref, trSlice)
		if err != nil {
			return nil, fmt.Errorf("wacogo/host: AddType %q runtime type: %w", tr.exportName, err)
		}
		typeValues[tr] = t
	}

	// Keep each live scope's type space lexical while retaining stable indexes:
	// every scope gets the existing resource-slot prefix followed by only its
	// own AddType declarations. The root alone populates the resource prefix.
	scopeTypeSlots := make(map[*scopeRuntime][]core.InstanceTypeSlot)
	var assignScopeTypeSlots func(sr *scopeRuntime, root bool)
	assignScopeTypeSlots = func(sr *scopeRuntime, root bool) {
		slots := make([]core.InstanceTypeSlot, typeSlotBase+len(sr.types))
		if root {
			copy(slots[:typeSlotBase], rootResourceSlots)
		}
		for _, tr := range sr.types {
			idx := typeSlotBase + int(tr.slot)
			slots[idx] = core.InstanceTypeSlot{Name: tr.exportName, Type: typeValues[tr]}
		}
		scopeTypeSlots[sr] = slots
		for _, child := range sr.nested {
			assignScopeTypeSlots(child, false)
		}
	}
	assignScopeTypeSlots(c.root, true)

	coreInst, err := core.NewInstance(c.engine, &core.InstanceSpec{
		Modules:            []api.Module{stubMod, hostMod},
		ParserInstanceType: wpInst,
		ExternTable:        h.extTable,
		BuildExports: func(inst *core.ComponentInstance) ([]core.InstanceTypeSlot, []core.InstanceExport, error) {
			// Build a flat funcExports slice parallel to c.allFuncs
			// so wpFuncTypes[i] matches.
			funcExports := make([]*core.ExportedFunc, len(c.allFuncs))
			for i, fr := range c.allFuncs {
				coreFn := stubMod.ExportedFunction(fr.stubExportName)
				if coreFn == nil {
					return nil, nil, fmt.Errorf("wacogo/host: stub missing export %q", fr.stubExportName)
				}
				coreFT, err := buildCoreFuncType(fr.userFT, trSlice)
				if err != nil {
					return nil, nil, fmt.Errorf("wacogo/host: func %q: %w", fr.exportName, err)
				}
				callee := canon.Callee{
					CallSide: canon.CallSide{
						Instance:       core.InstanceAsCanon(inst),
						Memory:         stubMemory,
						Realloc:        reallocAPI,
						StringEncoding: canon.EncUTF8,
					},
					CoreFunc: coreFn,
				}
				binding := canon.NewCallBinding(
					core.FuncTypeParamsAsCanon(coreFT),
					core.FuncTypeResultsAsCanon(coreFT),
					callee,
				)
				funcExports[i] = core.NewExportedFunc(
					fr.exportName,
					core.NewFunc(coreFT, binding),
					wpFuncTypes[i],
				)
			}

			// scopeExports walks one *scopeRuntime and emits the
			// InstanceExport list for it, recursing for nested
			// instances. Nested instances become bare *ComponentInstance
			// shells (no modules / no parser type / no extern table).
			var scopeExports func(sr *scopeRuntime) ([]core.InstanceExport, error)
			scopeExports = func(sr *scopeRuntime) ([]core.InstanceExport, error) {
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
				for _, rr := range sr.resources {
					out = append(out, core.InstanceExport{
						Name: rr.exportName, Kind: core.InstanceKindType,
						TypeIdx: uint32(resourceTRIdx[rr]),
					})
				}
				for _, ar := range sr.aliases {
					out = append(out, core.InstanceExport{
						Name: ar.exportName, Kind: core.InstanceKindType,
						TypeIdx: uint32(resourceTRIdx[ar.target]),
					})
				}
				for _, cm := range sr.coreModules {
					out = append(out, core.InstanceExport{
						Name:           cm.exportName,
						Kind:           core.InstanceKindCoreModule,
						CompiledModule: cm.compiled,
					})
				}
				for _, tr := range sr.types {
					if tr.exportName == "" {
						continue
					}
					out = append(out, core.InstanceExport{
						Name: tr.exportName, Kind: core.InstanceKindType,
						TypeIdx: uint32(typeSlotBase) + tr.slot,
					})
				}
				for _, child := range sr.nested {
					childExports, err := scopeExports(child)
					if err != nil {
						return nil, err
					}
					childExportsCopy := childExports
					childInst, err := core.NewInstance(c.engine, &core.InstanceSpec{
						BuildExports: func(*core.ComponentInstance) ([]core.InstanceTypeSlot, []core.InstanceExport, error) {
							return scopeTypeSlots[child], childExportsCopy, nil
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

			rootExports, err := scopeExports(c.root)
			if err != nil {
				return nil, nil, err
			}
			// Append resourceRef exports for the root only.
			for i, rrd := range c.resourceRefs {
				rootExports = append(rootExports, core.InstanceExport{
					Name: rrd.name, Kind: core.InstanceKindType,
					TypeIdx: uint32(len(c.allResources) + i),
				})
			}
			return scopeTypeSlots[c.root], rootExports, nil
		},
	})
	if err != nil {
		_ = stubMod.Close(ctx)
		_ = hostMod.Close(ctx)
		return nil, err
	}
	h.core = coreInst
	h.hostCallCC = core.NewCallContext(h.core, nil, stubMemory, stubRealloc)

	return h, nil
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

// buildComponentWpType pushes every wasmparser type needed by the host
// Component into its arena and constructs the recursively advertised exports,
// using each ResourceTypeRef's placeholderRID for refs. Returns a frozen
// *ComponentType handle plus the per-func *FuncType handle slice, indexed
// parallel to c.allFuncs. Called once from Builder.Build; never reads
// per-Instantiate state.
func buildComponentWpType(c *Component) (*wasmparser.ComponentType, []*wasmparser.FuncType, error) {
	htb := c.arena.HostTypeBuilder()
	xt := newInstTranslator(htb)

	for _, rr := range c.allResources {
		xt.resResID[rr.rt] = htb.AllocResourceID()
	}
	for _, rrd := range c.resourceRefs {
		xt.refResID[rrd.ref] = rrd.placeholderRID
	}

	// Translate every AddType declaration, including anonymous structural
	// declarations, before constructing exports. The TypeRef cache guarantees
	// that function signatures and named exports refer to the same arena entry.
	typeValTypes := make(map[*typeRuntime]wasmparser.ValTypeDesc, len(c.allTypes))
	for _, tr := range c.allTypes {
		vt, err := xt.instTranslateValType(tr.ref)
		if err != nil {
			return nil, nil, fmt.Errorf("wacogo/host: AddType %q parser type: %w", tr.exportName, err)
		}
		if vt.IsPrimitive {
			return nil, nil, fmt.Errorf("wacogo/host: AddType %q unexpectedly resolved to a primitive", tr.exportName)
		}
		typeValTypes[tr] = vt
	}

	// Translate every func signature in build order so wpFuncTypes is
	// indexed parallel to c.allFuncs.
	wpFuncTypes := make([]*wasmparser.FuncType, len(c.allFuncs))
	funcIDs := make([]wasmparser.ComponentFuncTypeID, len(c.allFuncs))
	funcIdx := map[*funcRuntime]int{}
	for i, fr := range c.allFuncs {
		fd, err := xt.instTranslateFuncType(fr.userFT)
		if err != nil {
			return nil, nil, fmt.Errorf("wacogo/host: func %q: %w", fr.exportName, err)
		}
		id := htb.PushFuncType(fd)
		funcIDs[i] = id
		wpFuncTypes[i] = htb.NewFuncTypeHandle(id)
		funcIdx[fr] = i
	}

	// Recursively build the wasmparser exports map for one scope. For
	// nested scopes this pushes a sub-ComponentTypeDesc + sub-
	// InstanceTypeDesc into the arena and references it via
	// EntityInstance{InstID:...} on the parent.
	var buildScopeExports func(sr *scopeRuntime) (map[string]wasmparser.ComponentEntityType, []string)
	buildScopeExports = func(sr *scopeRuntime) (map[string]wasmparser.ComponentEntityType, []string) {
		exp := make(map[string]wasmparser.ComponentEntityType,
			len(sr.funcs)+len(sr.types)+len(sr.resources)+len(sr.aliases)+len(sr.coreModules)+len(sr.nested))
		var order []string
		for _, fr := range sr.funcs {
			exp[fr.exportName] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityFunc, FuncID: funcIDs[funcIdx[fr]],
			}
			order = append(order, fr.exportName)
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
		for _, tr := range sr.types {
			if tr.exportName == "" {
				continue
			}
			vt := typeValTypes[tr]
			exp[tr.exportName] = wasmparser.ComponentEntityType{
				Kind: wasmparser.EntityType,
				TypeRef: wasmparser.ComponentAnyTypeID{
					Kind:  wasmparser.AnyTypeDefined,
					Index: uint32(vt.TypeID),
				},
			}
			order = append(order, tr.exportName)
		}
		for _, child := range sr.nested {
			childExp, childOrder := buildScopeExports(child)
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
		return exp, order
	}

	exports, exportOrder := buildScopeExports(c.root)
	for _, rrd := range c.resourceRefs {
		exports[rrd.name] = wasmparser.ComponentEntityType{
			Kind: wasmparser.EntityType,
			TypeRef: wasmparser.ComponentAnyTypeID{
				Kind:  wasmparser.AnyTypeResource,
				ResID: xt.refResID[rrd.ref],
			},
		}
		exportOrder = append(exportOrder, rrd.name)
	}

	ctID := htb.PushComponentType(wasmparser.ComponentTypeDesc{
		Imports:     map[string]wasmparser.ComponentEntityType{},
		ImportOrder: nil,
		Exports:     exports,
		ExportOrder: exportOrder,
	})
	return htb.NewComponentTypeHandle(ctID), wpFuncTypes, nil
}

// buildInstanceOrigins constructs the resourceOrigin map binding each
// ResourceTypeRef's placeholderRID to the corresponding lender
// instance and that lender's exported ResourceID. Called once per
// Instantiate; performs no arena writes.
func buildInstanceOrigins(c *Component, opts []resourceFromOpt) (map[wasmparser.ResourceID]wasmparser.ResourceOriginSpec, error) {
	if len(c.resourceRefs) == 0 {
		return nil, nil
	}
	optByRef := make(map[*ResourceTypeRef]resourceFromOpt, len(opts))
	for _, o := range opts {
		optByRef[o.ref] = o
	}
	origins := make(map[wasmparser.ResourceID]wasmparser.ResourceOriginSpec, len(c.resourceRefs))
	for _, rrd := range c.resourceRefs {
		opt, ok := optByRef[rrd.ref]
		if !ok {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q: no lender supplied", rrd.name)
		}
		lenderWp := opt.lender.ParserInstanceType()
		if lenderWp == nil {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q: lender has no wasmparser type info", rrd.name)
		}
		lenderRID, ok := lenderWp.ExportedResourceID(opt.lenderExport)
		if !ok {
			return nil, fmt.Errorf("wacogo/host: ResourceTypeRef %q: lender does not export resource type %q", rrd.name, opt.lenderExport)
		}
		origins[rrd.placeholderRID] = wasmparser.ResourceOriginSpec{
			Lender:      lenderWp,
			LenderResID: lenderRID,
		}
	}
	return origins, nil
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
