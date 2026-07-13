package core

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

var instanceCounter atomic.Uint64

// instantiationState holds the runtime index spaces for a component being
// instantiated. Plan steps append to these slices as they execute.
//
// The spec's component type index space (spec: type) is not tracked here —
// all type information needed at runtime is resolved at load time and embedded
// directly in plan steps (e.g. planLift.funcType, planImportFunc.funcType).
type instantiationState struct {
	inst    *ComponentInstance // the instance being built
	imports map[string]any     // one of *Func, *ComponentInstance, *CompiledModule, *Component, Type, Val

	// Core index spaces — populated by plan step execution (aliases,
	// instantiations, canon.lower, resource builtins).
	coreInstances []api.Module // spec: core:instance
	coreFuncs     []coreFunc   // spec: core:func
	coreMemories  []coreMemory // spec: core:memory
	coreTables    []coreTable  // spec: core:table
	coreGlobals   []coreGlobal // spec: core:global

	// compiledModules is the core module index space (spec: core:module).
	// Starts as a copy of component.compiledModules and grows as module
	// imports and aliases are resolved.
	compiledModules []CompiledModule

	// subComponents is the component index space (spec: component). Starts
	// as a copy of component.subComponents and grows as component imports
	// are resolved.
	subComponents []*Component

	// Component index spaces
	funcs              []*ExportedFunc      // spec: func
	componentInstances []*ComponentInstance // spec: instance
	values             []Val                // spec: value

	// funcExportInfo, indexed by component func-index, supplies the
	// export name and wasmparser function-type handle for funcs that
	// will become exports of this instance. Populated before the plan
	// runs so buildLiftedFunc can stamp them at construction. Funcs not
	// destined for export have a zero entry (empty Name, nil type).
	funcExportInfo []funcExportInfo

	// Auxiliary adapters created during instantiation (e.g., cross-component
	// adapter host modules and stub wrappers). These are tracked separately
	// from coreInstances so they don't shift the core instance index space.
	auxiliaryAdapters []*canon.CallAdapter

	// parentState provides access to the parent component's instantiation state,
	// used to resolve outer aliases at runtime. Nil for top-level instantiations.
	parentState *instantiationState
}

type coreFunc struct {
	instance api.Module
	name     string
}

// funcExportInfo carries the export-time identity of a component func
// so it can be stamped onto its *ExportedFunc at construction.
type funcExportInfo struct {
	name           string
	parserFuncType *wasmparser.FuncType
}

type coreMemory struct {
	instance api.Module
	name     string
}

type coreTable struct {
	instance api.Module
	name     string
}

type coreGlobal struct {
	instance api.Module
	name     string
	valType  byte
	mutable  bool
}

func (e *Engine) instantiate(ctx context.Context, component *Component, opts ...InstantiateOption) (*ComponentInstance, error) {
	if e.closed.Load() {
		return nil, ErrEngineClosed
	}
	cfg := applyInstantiateOptions(opts)
	if err := component.checkInstantiationImports(cfg); err != nil {
		return nil, err
	}

	inst, err := e.instantiateWithParentCfg(ctx, component, nil, cfg)
	if err != nil {
		return nil, err
	}
	// Mint a fresh instance-type handle for this top-level instance. Each
	// top-level Instantiate call gets its own fresh ResourceIDs for the
	// component's exported resources. Sub-component instances (created via
	// instantiateWithParent with a non-nil parent) do NOT mint one — their
	// type identity is handled by the validator-time check.
	if component.wpType != nil {
		wpInst := component.wpType.NewInstance()
		// Record, for each exported resource that is an alias of a
		// resource defined by a supplied instance import, the lender
		// instance and its arena ResID. CheckInstantiation walks this
		// map so chained re-exports share scope with the originator.
		suppliedInstanceImports := make(map[string]*wasmparser.InstanceType)
		for name, v := range cfg.imports {
			ci, ok := v.(*ComponentInstance)
			if !ok || ci == nil || ci.wpInstance == nil {
				continue
			}
			suppliedInstanceImports[name] = ci.wpInstance
		}
		if origins := component.wpType.ComputeResourceOrigins(suppliedInstanceImports); origins != nil {
			wpInst.SetResourceOrigin(origins)
		}
		inst.wpInstance = wpInst
	}
	return inst, nil
}

func (e *Engine) instantiateWithParent(ctx context.Context, component *Component, parentState *instantiationState, opts ...InstantiateOption) (*ComponentInstance, error) {
	cfg := applyInstantiateOptions(opts)
	return e.instantiateWithParentCfg(ctx, component, parentState, cfg)
}

// checkInstantiationImports is the shared, non-executing import validation
// path for Component.CheckInstantiation and top-level Instantiate. It checks
// required-import presence and runtime kind before asking wasmparser to check
// providers that carry parser type metadata.
func (component *Component) checkInstantiationImports(cfg *instantiateConfig) error {
	args := make(map[string]any, len(component.imports))
	for _, imp := range component.imports {
		name := wasmparser.CanonicalizeImportName(imp.Name)
		provider, ok := cfg.imports[name]
		if !ok || isNilImportProvider(provider) {
			return fmt.Errorf("wacogo: import %q: was not found", imp.Name)
		}
		if err := validateImportKind(imp, provider); err != nil {
			return err
		}

		switch p := provider.(type) {
		case *ComponentInstance:
			if p.wpInstance != nil {
				args[name] = p.wpInstance
			}
		case *ExportedFunc:
			if ft := p.ParserFunctionType(); ft != nil {
				args[name] = ft
			}
		case *CompiledModule:
			if p.wpModuleType != nil {
				args[name] = p.wpModuleType
			}
		case *Component:
			if p.wpType != nil {
				args[name] = p.wpType
			}
		}
	}

	if component.wpType != nil {
		if err := component.wpType.CheckInstantiation(args); err != nil {
			return fmt.Errorf("wacogo: instantiate: %w", err)
		}
	}
	return nil
}

func isNilImportProvider(provider any) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// instantiateWithParentCfg is the shared body of instantiateWithParent, taking
// an already-parsed config so callers that need to inspect imports before
// running the plan don't have to apply InstantiateOptions twice.
func (e *Engine) instantiateWithParentCfg(ctx context.Context, component *Component, parentState *instantiationState, cfg *instantiateConfig) (*ComponentInstance, error) {
	// Copy the component's compiled modules and sub-components into the
	// instantiation state so that module/component imports can extend them.
	compiledModules := make([]CompiledModule, len(component.compiledModules))
	copy(compiledModules, component.compiledModules)
	subComponents := make([]*Component, len(component.subComponents))
	copy(subComponents, component.subComponents)

	// Resolve outer closures from the parent state. This fills in compiled
	// modules and sub-components that were import placeholders at load time.
	if parentState != nil {
		for slotIdx, ref := range component.outerModuleClosures {
			target := parentState
			for i := uint32(1); i < ref.count; i++ {
				if target.parentState == nil {
					panic(fmt.Sprintf("wacogo: outer closure: count %d exceeds nesting depth", ref.count))
				}
				target = target.parentState
			}
			if int(ref.index) >= len(target.compiledModules) {
				panic(fmt.Sprintf("wacogo: outer closure module index %d out of range (have %d)", ref.index, len(target.compiledModules)))
			}
			compiledModules[slotIdx] = target.compiledModules[ref.index]
		}
		for slotIdx, ref := range component.outerComponentClosures {
			target := parentState
			for i := uint32(1); i < ref.count; i++ {
				if target.parentState == nil {
					break
				}
				target = target.parentState
			}
			if int(ref.index) >= len(target.subComponents) {
				panic(fmt.Sprintf("wacogo: outer closure component index %d out of range (have %d)", ref.index, len(target.subComponents)))
			}
			subComponents[slotIdx] = target.subComponents[ref.index]
		}
	}

	// Validate import kind compatibility before running any plan steps.
	if cfg.imports != nil {
		for _, imp := range component.imports {
			provider, ok := cfg.imports[wasmparser.CanonicalizeImportName(imp.Name)]
			if !ok {
				continue // Missing imports are handled by the plan step execution.
			}
			if err := validateImportKind(imp, provider); err != nil {
				return nil, err
			}
		}
	}

	inst := &ComponentInstance{
		engine:    e,
		component: component,
		exports:   make(map[string]exportEntry),
		types:     make([]Type, len(component.typeResolvers)),
		canLeave:  true,
	}
	inst.resources = NewResourceTable(inst)
	if parentState != nil {
		inst.parent = parentState.inst
	}
	state := &instantiationState{
		inst:            inst,
		imports:         cfg.imports,
		compiledModules: compiledModules,
		subComponents:   subComponents,
		parentState:     parentState,
		funcExportInfo:  buildFuncExportInfo(component.exports),
	}

	// Run the plan-step loop with the instance locked against reentrant
	// entry. Core modules' start functions may call back through
	// lowered→lifted functions during instantiation; the reentrance guard
	// makes that trap instead of recursing.
	runErr := inst.RunInComponent(ctx, func() error {
		for _, step := range component.plan {
			if err := step.execute(ctx, state); err != nil {
				return err
			}
		}
		return nil
	})
	if runErr != nil {
		// Clean up any already-instantiated modules.
		for _, m := range state.coreInstances {
			_ = m.Close(ctx)
		}
		for _, a := range state.auxiliaryAdapters {
			_ = a.Close(ctx)
		}
		return nil, runErr
	}

	// Resolve closures in sub-components. Sub-components that have outer
	// alias closures referencing our module/component index spaces need
	// their definitions updated with the actual values.
	state.resolveSubComponentClosures()

	// Wire up component exports.
	for _, exp := range component.exports {
		switch exp.Kind {
		case SortFunc:
			if int(exp.Index) >= len(state.funcs) {
				return nil, fmt.Errorf("wacogo: export %q references component func %d, but only %d exist", exp.Name, exp.Index, len(state.funcs))
			}
			fn := state.funcs[exp.Index]
			if fn == nil {
				return nil, fmt.Errorf("wacogo: export %q: missing *Func", exp.Name)
			}
			if fn.instance == nil {
				fn.instance = inst
			}
			inst.exports[exp.Name] = exportEntry{
				kind:    SortFunc,
				funcVal: fn,
			}
		case SortCoreModule:
			if int(exp.Index) >= len(state.compiledModules) {
				panic(fmt.Sprintf("wacogo: export %q: core module index %d out of range (have %d)", exp.Name, exp.Index, len(state.compiledModules)))
			}
			cm := state.compiledModules[exp.Index]
			inst.exports[exp.Name] = exportEntry{
				kind:           SortCoreModule,
				compiledModule: &cm,
			}
		case SortComponent:
			if int(exp.Index) >= len(state.subComponents) {
				panic(fmt.Sprintf("wacogo: export %q: component index %d out of range (have %d)", exp.Name, exp.Index, len(state.subComponents)))
			}
			inst.exports[exp.Name] = exportEntry{
				kind:      SortComponent,
				component: state.subComponents[exp.Index],
			}
		case SortInstance:
			if int(exp.Index) >= len(state.componentInstances) {
				panic(fmt.Sprintf("wacogo: export %q: instance index %d out of range (have %d)", exp.Name, exp.Index, len(state.componentInstances)))
			}
			inst.exports[exp.Name] = exportEntry{
				kind:     SortInstance,
				instance: state.componentInstances[exp.Index],
			}
		case SortType:
			// exp.Index is a TypeID in the component type index space.
			// Store it so exportedType(name) can look up the resolved Type
			// in inst.types at call time.
			inst.exports[exp.Name] = exportEntry{
				kind:    SortType,
				typeIdx: exp.Index,
			}
		}
	}

	inst.coreInstances = state.coreInstances
	inst.auxiliaryAdapters = state.auxiliaryAdapters
	return inst, nil
}

// validateImportKind checks that a supplied arg's Go type matches the kind
// expected for the import. Returns an error on mismatch (including for sorts
// the runtime does not currently support via the public API, such as values).
func validateImportKind(imp ImportDesc, arg any) error {
	switch imp.Kind {
	case SortFunc:
		if _, ok := arg.(*ExportedFunc); !ok {
			return fmt.Errorf("wacogo: import %q: expected function found %s", imp.Name, importArgSpecKind(arg))
		}
	case SortCoreModule:
		if _, ok := arg.(*CompiledModule); !ok {
			return fmt.Errorf("wacogo: import %q: expected module found %s", imp.Name, importArgSpecKind(arg))
		}
	case SortComponent:
		if _, ok := arg.(*Component); !ok {
			return fmt.Errorf("wacogo: import %q: expected component found %s", imp.Name, importArgSpecKind(arg))
		}
	case SortInstance:
		if _, ok := arg.(*ComponentInstance); !ok {
			return fmt.Errorf("wacogo: import %q: expected instance found %s", imp.Name, importArgSpecKind(arg))
		}
	case SortType:
		if _, ok := arg.(Type); !ok {
			return fmt.Errorf("wacogo: import %q: expected type found %s", imp.Name, importArgSpecKind(arg))
		}
	case SortValue:
		if _, ok := arg.(Val); !ok {
			return fmt.Errorf("wacogo: import %q: expected value found %s", imp.Name, importArgSpecKind(arg))
		}
	default:
		return fmt.Errorf("wacogo: import %q: unsupported kind %s", imp.Name, sortName(imp.Kind))
	}
	return nil
}

// importArgSpecKind returns the spec-style kind name for a provider arg, used
// in import-kind mismatch error messages so the phrasing matches the wasm-tools
// component-model spec suite (e.g. "expected module found instance").
func importArgSpecKind(arg any) string {
	switch arg.(type) {
	case *ExportedFunc:
		return "function"
	case *CompiledModule:
		return "module"
	case *Component:
		return "component"
	case *ComponentInstance:
		return "instance"
	case Type:
		return "type"
	case Val:
		return "value"
	default:
		return fmt.Sprintf("%T", arg)
	}
}

// sortName returns a human-readable name for a Sort value.
func sortName(s Sort) string {
	switch s {
	case SortFunc:
		return "func"
	case SortCoreModule:
		return "module"
	case SortComponent:
		return "component"
	case SortInstance:
		return "instance"
	case SortType:
		return "type"
	case SortValue:
		return "value"
	default:
		return "unknown"
	}
}

func (step *planInstantiateModule) execute(ctx context.Context, s *instantiationState) error {
	if int(step.moduleIndex) >= len(s.compiledModules) {
		return fmt.Errorf("wacogo: instantiate: module index %d out of range", step.moduleIndex)
	}
	cm := s.compiledModules[step.moduleIndex]

	if cm.module == nil {
		return fmt.Errorf("wacogo: instantiate: module %d has nil compiled module", step.moduleIndex)
	}

	// Build import resolver: map import module names to core instances.
	importMap := make(map[string]api.Module)
	for _, binding := range step.imports {
		if binding.instanceKind == instanceKindCore {
			if int(binding.instanceIdx) >= len(s.coreInstances) {
				return fmt.Errorf("wacogo: instantiate: import binding references core instance %d, but only %d exist", binding.instanceIdx, len(s.coreInstances))
			}
			importMap[binding.exportName] = s.coreInstances[binding.instanceIdx]
		}
	}

	// Handle synthetic name for blank-import rewriting.
	if cm.syntheticName != "" {
		// If there's a binding for the empty string, also map the synthetic name.
		if inst, ok := importMap[""]; ok {
			importMap[cm.syntheticName] = inst
			delete(importMap, "")
		}
	}

	resolver := func(name string) api.Module {
		return importMap[name]
	}

	resolverCtx := experimental.WithImportResolver(ctx, resolver)

	// Use a unique module name to avoid wazero collisions.
	modName := fmt.Sprintf("__wacogo_inst_%d", instanceCounter.Add(1))
	modCfg := wazero.NewModuleConfig().WithName(modName)

	inst, err := s.inst.engine.runtime.InstantiateModule(resolverCtx, cm.module, modCfg)
	if err != nil {
		return fmt.Errorf("wacogo: instantiate module %d: %w", step.moduleIndex, err)
	}

	s.coreInstances = append(s.coreInstances, inst)
	return nil
}

func (step *planAlias) execute(_ context.Context, s *instantiationState) error {
	switch src := step.source.(type) {
	case aliasCoreExport:
		if int(src.instanceIdx) >= len(s.coreInstances) {
			return fmt.Errorf("wacogo: alias: core instance %d out of range", src.instanceIdx)
		}
		inst := s.coreInstances[src.instanceIdx]
		switch step.targetSort {
		case SortCoreFunc:
			s.coreFuncs = append(s.coreFuncs, coreFunc{instance: inst, name: src.name})
		case SortCoreMemory:
			s.coreMemories = append(s.coreMemories, coreMemory{instance: inst, name: src.name})
		case SortCoreTable:
			s.coreTables = append(s.coreTables, coreTable{instance: inst, name: src.name})
		case SortCoreGlobal:
			g := inst.ExportedGlobal(src.name)
			if g == nil {
				return fmt.Errorf("wacogo: alias: global %q not found on core instance %d", src.name, src.instanceIdx)
			}
			_, mutable := g.(api.MutableGlobal)
			s.coreGlobals = append(s.coreGlobals, coreGlobal{
				instance: inst,
				name:     src.name,
				valType:  byte(g.Type()),
				mutable:  mutable,
			})
		}
	case aliasExport:
		if int(src.instanceIdx) >= len(s.componentInstances) {
			return fmt.Errorf("wacogo: alias export: component instance %d out of range (have %d)", src.instanceIdx, len(s.componentInstances))
		}
		inst := s.componentInstances[src.instanceIdx]
		switch step.targetSort {
		case SortFunc:
			entry, ok := inst.exports[src.name]
			if !ok || entry.kind != SortFunc {
				return fmt.Errorf("wacogo: alias export: %q not found as func in component instance %d", src.name, src.instanceIdx)
			}
			s.funcs = append(s.funcs, entry.funcVal)
		case SortInstance:
			sub := inst.ExportedInstance(src.name)
			if sub == nil {
				return fmt.Errorf("wacogo: alias export: %q not found as instance in component instance %d", src.name, src.instanceIdx)
			}
			s.componentInstances = append(s.componentInstances, sub)
		case SortCoreModule:
			cm := inst.ExportedModule(src.name)
			if cm == nil {
				return fmt.Errorf("wacogo: alias export: %q not found as core module in component instance %d", src.name, src.instanceIdx)
			}
			if step.targetIndex >= 0 {
				if step.targetIndex >= len(s.compiledModules) {
					panic(fmt.Sprintf("wacogo: alias export target module index %d out of range (have %d)", step.targetIndex, len(s.compiledModules)))
				}
				s.compiledModules[step.targetIndex] = *cm
			} else {
				s.compiledModules = append(s.compiledModules, *cm)
			}
		case SortComponent:
			comp := inst.ExportedComponent(src.name)
			if comp == nil {
				return fmt.Errorf("wacogo: alias export: %q not found as component in component instance %d", src.name, src.instanceIdx)
			}
			if step.targetIndex >= 0 {
				if step.targetIndex >= len(s.subComponents) {
					panic(fmt.Sprintf("wacogo: alias export target component index %d out of range (have %d)", step.targetIndex, len(s.subComponents)))
				}
				s.subComponents[step.targetIndex] = comp
			} else {
				s.subComponents = append(s.subComponents, comp)
			}
		default:
			// Other sorts (type, value) — not yet handled.
		}
	case aliasOuter:
		// Resolve outer aliases by walking the parent instantiation state chain.
		target := s
		for range uint32(src.count) {
			if target.parentState == nil {
				return fmt.Errorf("wacogo: outer alias: count %d exceeds nesting depth", src.count)
			}
			target = target.parentState
		}
		switch step.targetSort {
		case SortCoreModule:
			if int(src.index) >= len(target.compiledModules) {
				return fmt.Errorf("wacogo: outer alias: core module %d out of range (have %d)", src.index, len(target.compiledModules))
			}
			cm := target.compiledModules[src.index]
			if step.targetIndex >= 0 {
				if step.targetIndex >= len(s.compiledModules) {
					panic(fmt.Sprintf("wacogo: outer alias target module index %d out of range (have %d)", step.targetIndex, len(s.compiledModules)))
				}
				s.compiledModules[step.targetIndex] = cm
			} else {
				s.compiledModules = append(s.compiledModules, cm)
			}
		case SortComponent:
			if int(src.index) >= len(target.subComponents) {
				return fmt.Errorf("wacogo: outer alias: component %d out of range (have %d)", src.index, len(target.subComponents))
			}
			comp := target.subComponents[src.index]
			if step.targetIndex >= 0 {
				if step.targetIndex >= len(s.subComponents) {
					panic(fmt.Sprintf("wacogo: outer alias target component index %d out of range (have %d)", step.targetIndex, len(s.subComponents)))
				}
				s.subComponents[step.targetIndex] = comp
			} else {
				s.subComponents = append(s.subComponents, comp)
			}
		}
	}
	return nil
}

// resolveSubComponentClosures clones and updates sub-components that have
// outer alias closures referencing this component's module/component slots.
// This ensures that when a sub-component is exported, it carries the actual
// compiled modules/sub-components rather than nil placeholders.
func (s *instantiationState) resolveSubComponentClosures() {
	for i, subComp := range s.subComponents {
		if subComp == nil {
			continue
		}
		needsClone := false
		for _, ref := range subComp.outerModuleClosures {
			if ref.count == 1 {
				needsClone = true
				break
			}
		}
		if !needsClone {
			for _, ref := range subComp.outerComponentClosures {
				if ref.count == 1 {
					needsClone = true
					break
				}
			}
		}
		if !needsClone {
			continue
		}
		// Clone the sub-component with resolved closures.
		cloned := *subComp
		cloned.compiledModules = make([]CompiledModule, len(subComp.compiledModules))
		copy(cloned.compiledModules, subComp.compiledModules)
		cloned.subComponents = make([]*Component, len(subComp.subComponents))
		copy(cloned.subComponents, subComp.subComponents)

		for slotIdx, ref := range subComp.outerModuleClosures {
			if ref.count == 1 {
				if int(ref.index) >= len(s.compiledModules) {
					panic(fmt.Sprintf("wacogo: closure resolution module index %d out of range (have %d)", ref.index, len(s.compiledModules)))
				}
				cloned.compiledModules[slotIdx] = s.compiledModules[ref.index]
			}
		}
		for slotIdx, ref := range subComp.outerComponentClosures {
			if ref.count == 1 {
				if int(ref.index) >= len(s.subComponents) {
					panic(fmt.Sprintf("wacogo: closure resolution component index %d out of range (have %d)", ref.index, len(s.subComponents)))
				}
				cloned.subComponents[slotIdx] = s.subComponents[ref.index]
			}
		}
		// Clear the closures that were just resolved (count==1).
		if len(subComp.outerModuleClosures) > 0 || len(subComp.outerComponentClosures) > 0 {
			newModClosures := make(map[int]outerRef)
			for slotIdx, ref := range subComp.outerModuleClosures {
				if ref.count > 1 {
					// Decrement count since we're one level closer now.
					newModClosures[slotIdx] = outerRef{count: ref.count - 1, index: ref.index}
				}
			}
			if len(newModClosures) > 0 {
				cloned.outerModuleClosures = newModClosures
			} else {
				cloned.outerModuleClosures = nil
			}
			newCompClosures := make(map[int]outerRef)
			for slotIdx, ref := range subComp.outerComponentClosures {
				if ref.count > 1 {
					newCompClosures[slotIdx] = outerRef{count: ref.count - 1, index: ref.index}
				}
			}
			if len(newCompClosures) > 0 {
				cloned.outerComponentClosures = newCompClosures
			} else {
				cloned.outerComponentClosures = nil
			}
		}

		s.subComponents[i] = &cloned

		// Recursively resolve closures in the cloned sub-component's sub-components.
		clonedState := &instantiationState{
			compiledModules: cloned.compiledModules,
			subComponents:   cloned.subComponents,
		}
		clonedState.resolveSubComponentClosures()
		cloned.subComponents = clonedState.subComponents
	}
}

// buildFuncExportInfo returns a slice indexed by component func-index,
// populated with export-time identity (name + wasmparser type handle)
// for each func that will be exported from this instance. The lift step
// uses this to stamp the identity onto the *ExportedFunc at
// construction time.
func buildFuncExportInfo(exports []ExportDesc) []funcExportInfo {
	var info []funcExportInfo
	for _, exp := range exports {
		if exp.Kind != SortFunc {
			continue
		}
		if int(exp.Index) >= len(info) {
			grown := make([]funcExportInfo, exp.Index+1)
			copy(grown, info)
			info = grown
		}
		info[exp.Index] = funcExportInfo{
			name:           exp.Name,
			parserFuncType: exp.ParserFunctionType,
		}
	}
	return info
}

// buildLiftedFunc resolves a lift's canonical options against the current
// instantiation state and returns a fully-constructed *ExportedFunc. The
// export-time name and wasmparser function-type handle (if any) are
// stamped from s.funcExportInfo at the upcoming index in s.funcs.
func (s *instantiationState) buildLiftedFunc(
	cf coreFunc,
	options canonicalOptions,
	funcType *FuncType,
) (*ExportedFunc, error) {
	coreFn := cf.instance.ExportedFunction(cf.name)
	if coreFn == nil {
		return nil, fmt.Errorf("wacogo: lift: core function %q not found in instance", cf.name)
	}
	var mem api.Memory
	if options.hasMemory {
		if int(options.memory) >= len(s.coreMemories) {
			return nil, fmt.Errorf("wacogo: lift: memory index %d out of range (have %d)", options.memory, len(s.coreMemories))
		}
		cm := s.coreMemories[options.memory]
		mem = cm.instance.ExportedMemory(cm.name)
		if mem == nil {
			return nil, fmt.Errorf("wacogo: lift: memory %q not exported by module", cm.name)
		}
	}
	var reallocFn api.Function
	var reallocModName, reallocFnExport string
	if options.hasRealloc {
		if int(options.realloc) >= len(s.coreFuncs) {
			return nil, fmt.Errorf("wacogo: lift: realloc index %d out of range (have %d)", options.realloc, len(s.coreFuncs))
		}
		rf := s.coreFuncs[options.realloc]
		reallocFn = rf.instance.ExportedFunction(rf.name)
		if reallocFn == nil {
			return nil, fmt.Errorf("wacogo: lift: realloc %q not exported by module", rf.name)
		}
		reallocModName = rf.instance.Name()
		reallocFnExport = rf.name
	}
	var postReturnFn api.Function
	if options.hasPostReturn {
		if int(options.postReturn) >= len(s.coreFuncs) {
			return nil, fmt.Errorf("wacogo: lift: post-return index %d out of range (have %d)", options.postReturn, len(s.coreFuncs))
		}
		pf := s.coreFuncs[options.postReturn]
		postReturnFn = pf.instance.ExportedFunction(pf.name)
		if postReturnFn == nil {
			return nil, fmt.Errorf("wacogo: lift: post-return %q not exported by module", pf.name)
		}
	}
	callee := canon.Callee{
		CallSide: canon.CallSide{
			Instance:        canonInstanceView{i: s.inst},
			Memory:          mem,
			Realloc:         reallocFn,
			StringEncoding:  options.stringEncoding,
			ReallocModName:  reallocModName,
			ReallocFnExport: reallocFnExport,
		},
		CoreFunc:   coreFn,
		PostReturn: postReturnFn,
	}
	fn := &Func{
		funcType: funcType,
		binding: canon.NewCallBinding(
			FuncTypeParamsAsCanon(funcType),
			FuncTypeResultsAsCanon(funcType),
			callee,
		),
	}
	idx := uint32(len(s.funcs))
	var info funcExportInfo
	if int(idx) < len(s.funcExportInfo) {
		info = s.funcExportInfo[idx]
	}
	return NewExportedFunc(info.name, fn, info.parserFuncType), nil
}

func (step *planLift) execute(_ context.Context, s *instantiationState) error {
	if int(step.coreFuncIndex) >= len(s.coreFuncs) {
		return fmt.Errorf("wacogo: lift: core func index %d out of range (have %d)", step.coreFuncIndex, len(s.coreFuncs))
	}
	ft, err := s.lookupFuncType(step.funcTypeID, "lift")
	if err != nil {
		return err
	}
	cf := s.coreFuncs[step.coreFuncIndex]
	fn, err := s.buildLiftedFunc(cf, step.options, ft)
	if err != nil {
		return err
	}
	s.funcs = append(s.funcs, fn)
	return nil
}

func (step *planImportFunc) execute(_ context.Context, s *instantiationState) error {
	arg, ok := s.imports[wasmparser.CanonicalizeImportName(step.name)]
	if !ok {
		return fmt.Errorf("wacogo: import %q: was not found", step.name)
	}
	providerFn, ok := arg.(*ExportedFunc)
	if !ok {
		return fmt.Errorf("wacogo: import %q: expected function found %s", step.name, importArgSpecKind(arg))
	}
	if _, err := s.lookupFuncType(step.funcTypeID, fmt.Sprintf("import %q", step.name)); err != nil {
		return err
	}
	s.funcs = append(s.funcs, providerFn)
	return nil
}

// lookupResourceType reads inst.types[typeID] and returns it as *TypeResource.
// Returns nil if the slot holds nil or a non-*TypeResource value — callers
// then pass nil through to the resourceTable, matching the pre-refactor
// discriminator-less behavior for slots whose type wasn't resolvable.
func lookupResourceType(s *instantiationState, typeID uint32) *TypeResource {
	if int(typeID) >= len(s.inst.types) {
		return nil
	}
	rt, _ := s.inst.types[typeID].(*TypeResource)
	return rt
}

// lookupFuncType reads inst.types[typeID] and returns it as *FuncType, or nil
// if the slot is unresolved (e.g., the FuncType references a type import that
// is not satisfied by the current carrier mechanism). Returns nil without
// error to preserve pre-typeResolvers behavior where plan steps carried a
// nil funcType and downstream code fell back accordingly. ctxDesc labels
// the caller for out-of-range panics, which are programmer errors.
func (s *instantiationState) lookupFuncType(typeID uint32, ctxDesc string) (*FuncType, error) {
	if int(typeID) >= len(s.inst.types) {
		return nil, fmt.Errorf("wacogo: %s: typeID %d out of range (have %d)", ctxDesc, typeID, len(s.inst.types))
	}
	t := s.inst.types[typeID]
	if t == nil {
		return nil, nil
	}
	ft, ok := t.(*FuncType)
	if !ok {
		return nil, fmt.Errorf("wacogo: %s: typeID %d is %T, not *FuncType", ctxDesc, typeID, t)
	}
	return ft, nil
}

func (step *planImportInstance) execute(_ context.Context, s *instantiationState) error {
	arg, ok := s.imports[wasmparser.CanonicalizeImportName(step.name)]
	if !ok {
		if step.runtimeEmpty {
			s.componentInstances = append(s.componentInstances, &ComponentInstance{})
			return nil
		}
		return fmt.Errorf("wacogo: import %q: was not found", step.name)
	}
	inst, ok := arg.(*ComponentInstance)
	if !ok {
		return fmt.Errorf("wacogo: import %q: expected instance found %s", step.name, importArgSpecKind(arg))
	}
	s.componentInstances = append(s.componentInstances, inst)
	return nil
}

func (step *planImportModule) execute(ctx context.Context, s *instantiationState) error {
	arg, ok := s.imports[wasmparser.CanonicalizeImportName(step.name)]
	if !ok {
		return fmt.Errorf("wacogo: import module %q: was not found", step.name)
	}
	cm, ok := arg.(*CompiledModule)
	if !ok {
		return fmt.Errorf("wacogo: import module %q: expected module found %s", step.name, importArgSpecKind(arg))
	}
	if int(step.index) >= len(s.compiledModules) {
		panic(fmt.Sprintf("wacogo: import module target index %d out of range (have %d)", step.index, len(s.compiledModules)))
	}
	s.compiledModules[step.index] = *cm
	return nil
}

func (step *planImportComponent) execute(_ context.Context, s *instantiationState) error {
	arg, ok := s.imports[wasmparser.CanonicalizeImportName(step.name)]
	if !ok {
		return fmt.Errorf("wacogo: import component %q: was not found", step.name)
	}
	comp, ok := arg.(*Component)
	if !ok {
		return fmt.Errorf("wacogo: import component %q: expected component found %s", step.name, importArgSpecKind(arg))
	}
	if int(step.index) >= len(s.subComponents) {
		panic(fmt.Sprintf("wacogo: import component target index %d out of range (have %d)", step.index, len(s.subComponents)))
	}
	s.subComponents[step.index] = comp
	return nil
}

func (step *planInstantiateComponent) execute(ctx context.Context, s *instantiationState) error {
	if int(step.componentIndex) >= len(s.subComponents) {
		return fmt.Errorf("wacogo: instantiate component: index %d out of range (have %d)", step.componentIndex, len(s.subComponents))
	}
	subComp := s.subComponents[step.componentIndex]

	// Build import options from the args. Each arg resolves into the parent's
	// index space for its sort and forwards the concrete Go value to the
	// sub-component via the typed WithXImport option.
	var opts []InstantiateOption
	for _, arg := range step.args {
		switch arg.kind {
		case SortInstance:
			if int(arg.index) >= len(s.componentInstances) {
				return fmt.Errorf("wacogo: instantiate component: arg %q references component instance %d, but only %d exist", arg.name, arg.index, len(s.componentInstances))
			}
			opts = append(opts, WithInstanceImport(arg.name, s.componentInstances[arg.index]))
		case SortFunc:
			if int(arg.index) >= len(s.funcs) {
				return fmt.Errorf("wacogo: instantiate component: arg %q references component func %d, but only %d exist", arg.name, arg.index, len(s.funcs))
			}
			fn := s.funcs[arg.index]
			if fn == nil {
				return fmt.Errorf("wacogo: instantiate component: arg %q has no *Func", arg.name)
			}
			opts = append(opts, WithFuncImport(arg.name, fn))
		case SortComponent:
			if int(arg.index) >= len(s.subComponents) {
				return fmt.Errorf("wacogo: instantiate component: arg %q references component %d, but only %d exist", arg.name, arg.index, len(s.subComponents))
			}
			opts = append(opts, WithComponentImport(arg.name, s.subComponents[arg.index]))
		case SortCoreModule:
			if int(arg.index) >= len(s.compiledModules) {
				return fmt.Errorf("wacogo: instantiate component: arg %q references module %d, but only %d exist", arg.name, arg.index, len(s.compiledModules))
			}
			cm := s.compiledModules[arg.index]
			opts = append(opts, WithModuleImport(arg.name, &cm))
		case SortType:
			if int(arg.index) >= len(s.inst.types) {
				return fmt.Errorf("wacogo: instantiate component: arg %q references type %d, but only %d resolved", arg.name, arg.index, len(s.inst.types))
			}
			t := s.inst.types[arg.index]
			if t == nil {
				return fmt.Errorf("wacogo: instantiate component: arg %q references unresolved type %d", arg.name, arg.index)
			}
			opts = append(opts, WithTypeImport(arg.name, t))
		case SortValue:
			// Value args not yet supported but don't block instantiation.
			continue
		default:
			return fmt.Errorf("wacogo: instantiate component: unsupported arg kind %d for %q", arg.kind, arg.name)
		}
	}

	inst, err := s.inst.engine.instantiateWithParent(ctx, subComp, s, opts...)
	if err != nil {
		return fmt.Errorf("wacogo: instantiate sub-component %d: %w", step.componentIndex, err)
	}
	s.componentInstances = append(s.componentInstances, inst)
	return nil
}

func (step *planComponentInstantiateFromExports) execute(_ context.Context, s *instantiationState) error {
	// Create a component instance from inline exports by assembling an export map.
	inst := &ComponentInstance{
		engine:   s.inst.engine,
		exports:  make(map[string]exportEntry),
		canLeave: true,
	}
	for _, exp := range step.exports {
		switch exp.kind {
		case SortFunc:
			if int(exp.index) >= len(s.funcs) {
				return fmt.Errorf("wacogo: component instantiate-from-exports: func %d out of range (have %d)", exp.index, len(s.funcs))
			}
			fn := s.funcs[exp.index]
			if fn == nil {
				return fmt.Errorf("wacogo: component instantiate-from-exports: missing *Func at %d", exp.index)
			}
			if fn.instance == nil {
				fn.instance = inst
			}
			inst.exports[exp.name] = exportEntry{
				kind:    SortFunc,
				funcVal: fn,
			}
		case SortInstance:
			if int(exp.index) >= len(s.componentInstances) {
				return fmt.Errorf("wacogo: component instantiate-from-exports: instance %d out of range (have %d)", exp.index, len(s.componentInstances))
			}
			inst.exports[exp.name] = exportEntry{
				kind:     SortInstance,
				instance: s.componentInstances[exp.index],
			}
		case SortCoreModule:
			if int(exp.index) >= len(s.compiledModules) {
				panic(fmt.Sprintf("wacogo: instantiate-from-exports: core module index %d out of range (have %d)", exp.index, len(s.compiledModules)))
			}
			cm := s.compiledModules[exp.index]
			inst.exports[exp.name] = exportEntry{
				kind:           SortCoreModule,
				compiledModule: &cm,
			}
		case SortComponent:
			if int(exp.index) >= len(s.subComponents) {
				panic(fmt.Sprintf("wacogo: instantiate-from-exports: component index %d out of range (have %d)", exp.index, len(s.subComponents)))
			}
			inst.exports[exp.name] = exportEntry{
				kind:      SortComponent,
				component: s.subComponents[exp.index],
			}
		default:
			// Other sorts (type, value) — not yet handled.
		}
	}
	s.componentInstances = append(s.componentInstances, inst)
	return nil
}

func (step *planLower) execute(ctx context.Context, s *instantiationState) error {
	funcIdx := step.funcRef.funcIdx
	if int(funcIdx) >= len(s.funcs) {
		return fmt.Errorf("wacogo: lower: component func index %d out of range (have %d)", funcIdx, len(s.funcs))
	}
	fn := s.funcs[funcIdx]
	if fn == nil {
		return fmt.Errorf("wacogo: lower: componentFunc %d missing *Func", funcIdx)
	}
	return s.execLowerWithGoAdapter(ctx, *step, fn)
}

func (step *planInstantiateFromExports) execute(ctx context.Context, s *instantiationState) error {
	// Create a wasm bridge module that imports the referenced core functions/memories
	// and re-exports them under the expected names. We must use a real wasm module
	// (not a host module) because wazero's ImportResolver only works with wasm
	// module instances, not host module instances, and we need to export non-functions.
	return s.execInstantiateFromExportsWithWasm(ctx, *step)
}

// execInstantiateFromExportsWithWasm handles the case where we need to
// re-export memories (which wazero host modules cannot do). Uses a tiny
// wasm bridge module that imports and re-exports the items.
func (s *instantiationState) execInstantiateFromExportsWithWasm(ctx context.Context, step planInstantiateFromExports) error {
	var b wasm.ModuleBuilder

	type bridgeImport struct {
		importModule string
		importName   string
	}
	var bridgeImports []bridgeImport

	bridgeID := instanceCounter.Add(1)

	for _, exp := range step.exports {
		switch exp.sort {
		case SortCoreFunc:
			if int(exp.index) >= len(s.coreFuncs) {
				return fmt.Errorf("wacogo: instantiate-from-exports: core func %d out of range (have %d)", exp.index, len(s.coreFuncs))
			}
			cf := s.coreFuncs[exp.index]
			fn := cf.instance.ExportedFunction(cf.name)
			if fn == nil {
				return fmt.Errorf("wacogo: instantiate-from-exports: core func %q not found in instance", cf.name)
			}

			def := fn.Definition()
			paramTypes := def.ParamTypes()
			resultTypes := def.ResultTypes()

			sig := wasm.FuncSig{
				Params:  make([]byte, len(paramTypes)),
				Results: make([]byte, len(resultTypes)),
			}
			for i, p := range paramTypes {
				sig.Params[i] = apiValTypeToWasm(p)
			}
			for i, r := range resultTypes {
				sig.Results[i] = apiValTypeToWasm(r)
			}

			importModName := fmt.Sprintf("__bridge_%d_%d", bridgeID, exp.index)
			importFuncIdx := b.AddImportFunc(importModName, cf.name, sig)
			b.AddExportFunc(exp.name, importFuncIdx)
			bridgeImports = append(bridgeImports, bridgeImport{importModule: importModName, importName: cf.name})

		case SortCoreMemory:
			if int(exp.index) >= len(s.coreMemories) {
				return fmt.Errorf("wacogo: instantiate-from-exports: core memory %d out of range (have %d)", exp.index, len(s.coreMemories))
			}
			cm := s.coreMemories[exp.index]

			importModName := fmt.Sprintf("__bridge_mem_%d_%d", bridgeID, exp.index)
			memIdx := b.AddImportMemory(importModName, cm.name, 0, 0)
			b.AddExportMemory(exp.name, memIdx)
			bridgeImports = append(bridgeImports, bridgeImport{importModule: importModName, importName: cm.name})

		case SortCoreTable:
			if int(exp.index) >= len(s.coreTables) {
				return fmt.Errorf("wacogo: instantiate-from-exports: core table %d out of range (have %d)", exp.index, len(s.coreTables))
			}
			ct := s.coreTables[exp.index]

			importModName := fmt.Sprintf("__bridge_tbl_%d_%d", bridgeID, exp.index)
			// Import with funcref type (0x70) and min 0 to be permissive.
			tblIdx := b.AddImportTable(importModName, ct.name, 0, 0, 0x70)
			b.AddExportTable(exp.name, tblIdx)
			bridgeImports = append(bridgeImports, bridgeImport{importModule: importModName, importName: ct.name})

		case SortCoreGlobal:
			if int(exp.index) >= len(s.coreGlobals) {
				return fmt.Errorf("wacogo: instantiate-from-exports: core global %d out of range (have %d)", exp.index, len(s.coreGlobals))
			}
			cg := s.coreGlobals[exp.index]

			importModName := fmt.Sprintf("__bridge_glb_%d_%d", bridgeID, exp.index)
			glbIdx := b.AddImportGlobal(importModName, cg.name, cg.valType, cg.mutable)
			b.AddExportGlobal(exp.name, glbIdx)
			bridgeImports = append(bridgeImports, bridgeImport{importModule: importModName, importName: cg.name})

		default:
			return fmt.Errorf("wacogo: instantiate-from-exports: unsupported sort %d", exp.sort)
		}
	}

	bridgeWasm := b.Encode()

	compiled, err := s.inst.engine.runtime.CompileModule(ctx, bridgeWasm)
	if err != nil {
		return fmt.Errorf("wacogo: instantiate-from-exports: compile bridge: %w", err)
	}

	importMap := make(map[string]api.Module)
	bridgeIdx := 0
	for _, exp := range step.exports {
		switch exp.sort {
		case SortCoreFunc:
			cf := s.coreFuncs[exp.index]
			importMap[bridgeImports[bridgeIdx].importModule] = cf.instance
		case SortCoreMemory:
			cm := s.coreMemories[exp.index]
			importMap[bridgeImports[bridgeIdx].importModule] = cm.instance
		case SortCoreTable:
			ct := s.coreTables[exp.index]
			importMap[bridgeImports[bridgeIdx].importModule] = ct.instance
		case SortCoreGlobal:
			cg := s.coreGlobals[exp.index]
			importMap[bridgeImports[bridgeIdx].importModule] = cg.instance
		}
		bridgeIdx++
	}

	resolver := func(name string) api.Module {
		return importMap[name]
	}

	resolverCtx := experimental.WithImportResolver(ctx, resolver)
	modName := fmt.Sprintf("__wacogo_bridge_%d", instanceCounter.Add(1))
	modCfg := wazero.NewModuleConfig().WithName(modName)

	inst, err := s.inst.engine.runtime.InstantiateModule(resolverCtx, compiled, modCfg)
	if err != nil {
		return fmt.Errorf("wacogo: instantiate-from-exports: instantiate bridge: %w", err)
	}

	s.coreInstances = append(s.coreInstances, inst)
	return nil
}

// planStart.execute returns an error because component-level value-typed
// args are not yet supported. When value support lands the implementation
// will decode s.values[argIdx] into Vals and route through fn.Call — the
// same path any Go caller uses.
func (step *planStart) execute(_ context.Context, _ *instantiationState) error {
	return fmt.Errorf("wacogo: start: component-level start functions are not yet supported")
}

// apiValTypeToWasm converts a wazero api.ValueType to the wasm byte constant.
func apiValTypeToWasm(vt api.ValueType) byte {
	switch vt {
	case api.ValueTypeI32:
		return wasm.ValI32
	case api.ValueTypeI64:
		return wasm.ValI64
	case api.ValueTypeF32:
		return wasm.ValF32
	case api.ValueTypeF64:
		return wasm.ValF64
	default:
		return wasm.ValI32
	}
}

func (step *planExportRebind) execute(_ context.Context, s *instantiationState) error {
	switch step.sort {
	case SortFunc:
		if int(step.sourceIndex) >= len(s.funcs) {
			return fmt.Errorf("wacogo: export rebind: func %d out of range (have %d)", step.sourceIndex, len(s.funcs))
		}
		s.funcs = append(s.funcs, s.funcs[step.sourceIndex])
	case SortInstance:
		if int(step.sourceIndex) >= len(s.componentInstances) {
			return fmt.Errorf("wacogo: export rebind: instance %d out of range (have %d)", step.sourceIndex, len(s.componentInstances))
		}
		// componentInstances is not pre-initialized from the load-time
		// scope (it starts empty and grows as instances are created), so
		// rebind always appends regardless of targetIndex.
		s.componentInstances = append(s.componentInstances, s.componentInstances[step.sourceIndex])
	case SortCoreModule:
		if int(step.sourceIndex) >= len(s.compiledModules) {
			return fmt.Errorf("wacogo: export rebind: module %d out of range (have %d)", step.sourceIndex, len(s.compiledModules))
		}
		if step.targetIndex >= 0 {
			s.compiledModules[step.targetIndex] = s.compiledModules[step.sourceIndex]
		} else {
			s.compiledModules = append(s.compiledModules, s.compiledModules[step.sourceIndex])
		}
	case SortComponent:
		if int(step.sourceIndex) >= len(s.subComponents) {
			return fmt.Errorf("wacogo: export rebind: component %d out of range (have %d)", step.sourceIndex, len(s.subComponents))
		}
		if step.targetIndex >= 0 {
			s.subComponents[step.targetIndex] = s.subComponents[step.sourceIndex]
		} else {
			s.subComponents = append(s.subComponents, s.subComponents[step.sourceIndex])
		}
	}
	return nil
}

func (step *planResolveType) execute(_ context.Context, s *instantiationState) error {
	c := s.inst.component
	if int(step.typeID) >= len(c.typeResolvers) {
		return fmt.Errorf("wacogo: planResolveType: typeID %d out of range (have %d)", step.typeID, len(c.typeResolvers))
	}
	r := c.typeResolvers[step.typeID]
	if r == nil {
		return fmt.Errorf("wacogo: planResolveType: nil resolver at %d", step.typeID)
	}
	// Sync the instance index space so instanceImportResolver can find
	// instances populated by preceding plan steps (imports and sub-instances).
	s.inst.instances = s.componentInstances
	s.inst.types[step.typeID] = r.resolve(&resolverCtx{inst: s.inst, imports: s.imports, state: s})
	return nil
}

// resolveDtor turns a dtor core-function index (from a resource-type
// declaration in the current component) into a callable api.Function
// against the live core instance. Returns nil if the index is out of
// range or the core function can't be located — the caller treats a
// nil dtor as "no destructor" and skips the call at drop time.
func (s *instantiationState) resolveDtor(funcIdx uint32) api.Function {
	if int(funcIdx) >= len(s.coreFuncs) {
		return nil
	}
	cf := s.coreFuncs[funcIdx]
	if cf.instance == nil {
		return nil
	}
	return cf.instance.ExportedFunction(cf.name)
}
