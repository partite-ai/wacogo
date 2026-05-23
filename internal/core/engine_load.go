package core

import (
	"context"
	"fmt"
	"io"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/partite-ai/wacogo/wasmparser"
)

// loadInstanceInfo tracks a component instance in the component instance index space.
type loadInstanceInfo struct {
	importName string
	component  *Component
}

// loadScope tracks the spec-defined index spaces of the component being loaded.
// Outer aliases reference parent scopes, so we pass them down during recursive loading.
//
// Not all spec-defined index spaces are tracked here:
//
//   - core:func, core:table, core:memory, core:global — only populated by
//     plan step execution at runtime (aliases of core instance exports,
//     canon.lower, resource builtins); no section processing reads them
//     during loading.
//   - core:instance — same; only created by instantiating core modules.
//   - core:type — delegated entirely to the wasmparser validator; not needed
//     for loading or instantiation.
//   - value — only produced by the start section at runtime.
type loadScope struct {
	parent *loadScope

	// compiledModules is the core module index space (spec: core:module).
	compiledModules []CompiledModule
	// subComponents is the component index space (spec: component).
	subComponents []*Component
	// componentInstances is the component instance index space (spec: instance).
	componentInstances []loadInstanceInfo

	// outerModuleClosures records deferred outer aliases for compiledModules slots
	// that reference import placeholders. Propagated to the Component.
	outerModuleClosures map[int]outerRef
	// outerComponentClosures records deferred outer aliases for subComponents slots
	// that reference import placeholders. Propagated to the Component.
	outerComponentClosures map[int]outerRef
}

// walkUp traverses count parent scopes. Panics if nesting depth is exceeded,
// since the validator guarantees the count is within bounds.
func (s *loadScope) walkUp(count uint32) *loadScope {
	target := s
	for range count {
		if target.parent == nil {
			panic(fmt.Sprintf("wacogo: outer alias count %d exceeds nesting depth", count))
		}
		target = target.parent
	}
	return target
}

// componentLoader accumulates mutable state during component loading.
type componentLoader struct {
	engine  *Engine
	scope   *loadScope
	parser  *wasmparser.ValidatingParser
	plan    []planStep
	exports []ExportDesc
	imports []ImportDesc

	// resourceDtors maps a resource TypeID to the core func index of its
	// destructor, captured at load time. Consumed by planResourceDrop to
	// wire the dtor callback at resource-drop time. A future refactor will
	// move the dtor onto *TypeResource so every resource carries its own
	// api.Function, eliminating this side-map; for now it's the simplest
	// way to preserve existing planResourceDrop semantics.
	resourceDtors map[uint32]uint32

	// typeResolvers accumulates one entry per allocated TypeID in declaration
	// order (type section entries, SortType aliases, and direct type imports).
	// Copied to Component.typeResolvers at buildComponent time. The current
	// length IS the next TypeID — no separate counter is maintained.
	typeResolvers []typeResolver

	// funcIsImport tracks, by component-func index, whether each func entry
	// originated from a direct (import ...) declaration. Used to reject
	// re-exports of imported funcs, which wasmtime's component runtime does
	// not implement.
	funcIsImport []bool
}

// allocType appends a resolver to the type index space and emits the
// corresponding planResolveType step. Returns the newly allocated TypeID.
func (l *componentLoader) allocType(r typeResolver) uint32 {
	idx := uint32(len(l.typeResolvers))
	l.typeResolvers = append(l.typeResolvers, r)
	l.plan = append(l.plan, &planResolveType{typeID: idx})
	return idx
}

// LoadComponent parses and compiles a component binary into a Component
// ready for instantiation.
func (e *Engine) LoadComponent(ctx context.Context, r io.Reader) (*Component, error) {
	vp := e.validator.NewValidatingParser(r)
	comp, err := e.loadComponentFromParser(ctx, vp, nil)
	if err != nil {
		return nil, err
	}
	comp.wpType = vp.TopLevelComponentType()
	return comp, nil
}

// loadComponentFromParser reads payloads from a single component's parser,
// recursively loading sub-components. Unlike ParseAll which flattens nesting,
// this tracks component boundaries so sub-components get their own Component.
func (e *Engine) loadComponentFromParser(ctx context.Context, parser *wasmparser.ValidatingParser, parentScope *loadScope) (*Component, error) {
	l := &componentLoader{
		engine: e,
		scope: &loadScope{
			parent:                 parentScope,
			outerModuleClosures:    make(map[int]outerRef),
			outerComponentClosures: make(map[int]outerRef),
		},
		parser:        parser,
		resourceDtors: make(map[uint32]uint32),
	}

	for {
		payload, err := parser.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("wacogo: parse: %w", err)
		}

		switch sec := payload.(type) {
		case *wasmparser.ValidatedModuleSectionPayload:
			if err := l.processModuleSection(ctx, sec); err != nil {
				return nil, err
			}
		case *wasmparser.ComponentSectionPayload:
			subComp, err := e.loadComponentFromParser(ctx, sec.ValidatingParser, l.scope)
			if err != nil {
				return nil, fmt.Errorf("wacogo: load sub-component: %w", err)
			}
			l.scope.subComponents = append(l.scope.subComponents, subComp)
		case *wasmparser.CoreInstanceSectionPayload:
			if err := l.processCoreInstanceSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ComponentInstanceSectionPayload:
			if err := l.processComponentInstanceSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ComponentTypeSectionPayload:
			if err := l.processTypeSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ComponentCanonicalSectionPayload:
			if err := l.processCanonicalSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ComponentAliasSectionPayload:
			if err := l.processAliasSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ValidatedComponentExportSectionPayload:
			if err := l.processExportSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ValidatedComponentImportSectionPayload:
			if err := l.processImportSection(sec); err != nil {
				return nil, err
			}
		case *wasmparser.ComponentStartSectionPayload:
			l.plan = append(l.plan, &planStart{
				funcIndex:   sec.Start.FuncIndex,
				args:        sec.Start.Args,
				resultCount: sec.Start.Results,
			})
		}
	}

	return l.buildComponent(parentScope)
}

func (l *componentLoader) processModuleSection(ctx context.Context, sec *wasmparser.ValidatedModuleSectionPayload) error {
	raw := sec.Raw()
	rewritten, syntheticName, err := wasm.RewriteBlankImportNames(raw.Data)
	if err != nil {
		return fmt.Errorf("wacogo: rewrite blank imports: %w", err)
	}
	compiled, err := l.engine.runtime.CompileModule(ctx, rewritten)
	if err != nil {
		return fmt.Errorf("wacogo: compile module: %w", err)
	}
	l.scope.compiledModules = append(l.scope.compiledModules, CompiledModule{
		module:        compiled,
		syntheticName: syntheticName,
		wpModuleType:  sec.ModuleType(),
	})
	return nil
}

func (l *componentLoader) processCoreInstanceSection(sec *wasmparser.CoreInstanceSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: core instance section: %w", err)
		}
		switch inst := item.(type) {
		case wasmparser.CoreInstantiate:
			bindings := make([]importBinding, len(inst.Args))
			for i, arg := range inst.Args {
				bindings[i] = importBinding{
					instanceKind: instanceKindCore,
					instanceIdx:  arg.Index,
					exportName:   arg.Name,
				}
			}
			l.plan = append(l.plan, &planInstantiateModule{
				moduleIndex: inst.ModuleIndex,
				imports:     bindings,
			})
		case wasmparser.CoreInstantiateFromExports:
			exports := make([]coreExportBinding, len(inst.Exports))
			for i, exp := range inst.Exports {
				exports[i] = coreExportBinding{
					name:  exp.Name,
					sort:  coreSortToSort(exp.Kind),
					index: exp.Index,
				}
			}
			l.plan = append(l.plan, &planInstantiateFromExports{
				exports: exports,
			})
		default:
			panic(fmt.Sprintf("wacogo: unknown core instance item %T", item))
		}
	}
	return nil
}

func (l *componentLoader) processComponentInstanceSection(sec *wasmparser.ComponentInstanceSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: component instance section: %w", err)
		}
		switch inst := item.(type) {
		case wasmparser.Instantiate:
			args := make([]instantiateArg, len(inst.Args))
			for i, arg := range inst.Args {
				args[i] = instantiateArg{
					name:  arg.Name,
					kind:  componentExternalKindToSort(arg.Kind),
					index: arg.Index,
				}
			}
			l.plan = append(l.plan, &planInstantiateComponent{
				componentIndex: inst.ComponentIndex,
				args:           args,
			})
			if int(inst.ComponentIndex) >= len(l.scope.subComponents) {
				panic(fmt.Sprintf("wacogo: component index %d out of range (have %d)", inst.ComponentIndex, len(l.scope.subComponents)))
			}
			l.scope.componentInstances = append(l.scope.componentInstances, loadInstanceInfo{
				component: l.scope.subComponents[inst.ComponentIndex],
			})
		case wasmparser.InstantiateFromExports:
			// Component-level instantiate-from-exports creates a
			// component instance from inline exports. Build the
			// exports map at instantiation time.
			l.plan = append(l.plan, &planComponentInstantiateFromExports{
				exports: convertComponentExports(inst.Exports),
			})
			l.scope.componentInstances = append(l.scope.componentInstances, loadInstanceInfo{})
		default:
			panic(fmt.Sprintf("wacogo: unknown component instance item %T", item))
		}
	}
	return nil
}

func (l *componentLoader) processTypeSection(sec *wasmparser.ComponentTypeSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: type section: %w", err)
		}
		// planResourceDrop reads the destructor from this map; *TypeResource
		// does not yet carry it directly.
		if rt, ok := item.(*wasmparser.ResourceType); ok && rt.Dtor.Valid {
			l.resourceDtors[uint32(len(l.typeResolvers))] = rt.Dtor.Value
		}
		l.allocType(buildComponentTypeResolver(item))
	}
	return nil
}

func (l *componentLoader) processCanonicalSection(sec *wasmparser.ComponentCanonicalSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: canonical section: %w", err)
		}
		switch cf := item.(type) {
		case wasmparser.CanonLift:
			l.plan = append(l.plan, &planLift{
				coreFuncIndex: cf.CoreFuncIndex,
				funcTypeID:    cf.TypeIndex,
				options:       mapCanonicalOptions(cf.Options),
			})
			l.funcIsImport = append(l.funcIsImport, false)
		case wasmparser.CanonLower:
			l.plan = append(l.plan, &planLower{
				funcRef: funcRef{funcIdx: cf.FuncIndex},
				options: mapCanonicalOptions(cf.Options),
			})
		case wasmparser.CanonResourceNew:
			l.plan = append(l.plan, &planResourceNew{typeID: cf.TypeIndex})
		case wasmparser.CanonResourceDrop:
			dtorIdx := -1
			if dtor, ok := l.resourceDtors[cf.TypeIndex]; ok {
				dtorIdx = int(dtor)
			}
			l.plan = append(l.plan, &planResourceDrop{typeID: cf.TypeIndex, dtorFuncIndex: dtorIdx})
		case wasmparser.CanonResourceRep:
			l.plan = append(l.plan, &planResourceRep{typeID: cf.TypeIndex})
		default:
			panic(fmt.Sprintf("wacogo: unknown canonical item %T", item))
		}
	}
	return nil
}

func (l *componentLoader) processAliasSection(sec *wasmparser.ComponentAliasSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: alias section: %w", err)
		}
		switch a := item.(type) {
		case wasmparser.AliasInstanceExport:
			l.processAliasInstanceExport(a)
		case wasmparser.AliasCoreInstanceExport:
			l.plan = append(l.plan, &planAlias{
				source:      aliasCoreExport{instanceIdx: a.Instance, name: a.Name},
				targetSort:  coreSortToSort(a.Kind),
				targetIndex: -1,
			})
		case wasmparser.AliasOuter:
			l.processAliasOuter(a)
		default:
			panic(fmt.Sprintf("wacogo: unknown alias item %T", item))
		}
	}
	return nil
}

func (l *componentLoader) processAliasInstanceExport(a wasmparser.AliasInstanceExport) {
	sort := componentExternalKindToSort(a.Kind)
	if sort == SortType {
		l.allocType(instanceImportResolver{instanceIdx: a.Instance, exportName: a.Name})
		return
	}
	alias := &planAlias{
		source:      aliasExport{instanceIdx: a.Instance, name: a.Name},
		targetSort:  sort,
		targetIndex: -1,
	}
	switch sort {
	case SortCoreModule:
		alias.targetIndex = len(l.scope.compiledModules)
		l.scope.compiledModules = append(l.scope.compiledModules, CompiledModule{})
	case SortComponent:
		alias.targetIndex = len(l.scope.subComponents)
		l.scope.subComponents = append(l.scope.subComponents, nil)
	case SortFunc:
		l.funcIsImport = append(l.funcIsImport, false)
	}
	l.plan = append(l.plan, alias)
}

func (l *componentLoader) processAliasOuter(a wasmparser.AliasOuter) {
	if a.Kind == wasmparser.OuterAliasKindCoreType {
		return
	}
	if a.Kind == wasmparser.OuterAliasKindType {
		l.allocType(aliasResolver{outerDepth: a.Count, typeIdx: a.Index})
		return
	}

	switch outerAliasKindToSort(a.Kind) {
	case SortCoreModule:
		target := l.scope.walkUp(a.Count)
		if int(a.Index) >= len(target.compiledModules) {
			panic(fmt.Sprintf("wacogo: outer alias: core module %d out of range in scope (have %d)", a.Index, len(target.compiledModules)))
		}
		cm := target.compiledModules[a.Index]
		slotIdx := len(l.scope.compiledModules)
		l.scope.compiledModules = append(l.scope.compiledModules, cm)
		if cm.module == nil {
			l.scope.outerModuleClosures[slotIdx] = outerRef{count: a.Count, index: a.Index}
		}
	case SortComponent:
		target := l.scope.walkUp(a.Count)
		if int(a.Index) >= len(target.subComponents) {
			panic(fmt.Sprintf("wacogo: outer alias: component %d out of range in scope (have %d)", a.Index, len(target.subComponents)))
		}
		comp := target.subComponents[a.Index]
		slotIdx := len(l.scope.subComponents)
		l.scope.subComponents = append(l.scope.subComponents, comp)
		if comp == nil {
			l.scope.outerComponentClosures[slotIdx] = outerRef{count: a.Count, index: a.Index}
		}
	default:
		l.plan = append(l.plan, &planAlias{
			source:      aliasOuter{count: a.Count, index: a.Index},
			targetSort:  outerAliasKindToSort(a.Kind),
			targetIndex: -1,
		})
	}
}

func (l *componentLoader) processExportSection(sec *wasmparser.ValidatedComponentExportSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: export section: %w", err)
		}
		sort := componentExternalKindToSort(item.Export.Kind)
		desc := ExportDesc{
			Name:  item.Export.Name.Name,
			Kind:  sort,
			Index: item.Export.Index,
		}
		if sort == SortFunc {
			desc.ParserFunctionType = item.FuncType()
		}
		l.exports = append(l.exports, desc)
		switch sort {
		case SortType:
			// Type exports allocate a new TypeID that aliases the exported type.
			l.allocType(indexResolver{idx: item.Export.Index})
		case SortFunc:
			// At the root component, re-exporting a directly imported function
			// is the host-boundary case wasmtime's component runtime does not
			// implement; reject it. Sub-components freely thread imports
			// through to exports — that pattern is resolved when the sub is
			// instantiated by its parent.
			if l.scope.parent == nil &&
				int(item.Export.Index) < len(l.funcIsImport) && l.funcIsImport[item.Export.Index] {
				return fmt.Errorf("component export `%s` is a reexport of an imported function which is not implemented",
					item.Export.Name.Name)
			}
			// Each exported function is also rebound into the enclosing
			// component's func index space. Mirror that at runtime so
			// subsequent plan steps resolve to the same coreFunc.
			l.plan = append(l.plan, &planExportRebind{
				sort:        SortFunc,
				sourceIndex: item.Export.Index,
				targetIndex: -1,
			})
			l.funcIsImport = append(l.funcIsImport, false)
		case SortInstance:
			// Carry the loader-time instance slot forward so outer
			// aliases and later inline instances resolve correctly.
			targetIdx := len(l.scope.componentInstances)
			if int(item.Export.Index) < len(l.scope.componentInstances) {
				l.scope.componentInstances = append(l.scope.componentInstances,
					l.scope.componentInstances[item.Export.Index])
			} else {
				l.scope.componentInstances = append(l.scope.componentInstances, loadInstanceInfo{})
			}
			l.plan = append(l.plan, &planExportRebind{
				sort:        SortInstance,
				sourceIndex: item.Export.Index,
				targetIndex: targetIdx,
			})
		case SortCoreModule:
			targetIdx := len(l.scope.compiledModules)
			if int(item.Export.Index) < len(l.scope.compiledModules) {
				l.scope.compiledModules = append(l.scope.compiledModules,
					l.scope.compiledModules[item.Export.Index])
			} else {
				l.scope.compiledModules = append(l.scope.compiledModules, CompiledModule{})
			}
			l.plan = append(l.plan, &planExportRebind{
				sort:        SortCoreModule,
				sourceIndex: item.Export.Index,
				targetIndex: targetIdx,
			})
		case SortComponent:
			targetIdx := len(l.scope.subComponents)
			if int(item.Export.Index) < len(l.scope.subComponents) {
				l.scope.subComponents = append(l.scope.subComponents,
					l.scope.subComponents[item.Export.Index])
			} else {
				l.scope.subComponents = append(l.scope.subComponents, nil)
			}
			l.plan = append(l.plan, &planExportRebind{
				sort:        SortComponent,
				sourceIndex: item.Export.Index,
				targetIndex: targetIdx,
			})
		}
	}
	return nil
}

func (l *componentLoader) processImportSection(sec *wasmparser.ValidatedComponentImportSectionPayload) error {
	for item, err := range sec.Items() {
		if err != nil {
			return fmt.Errorf("wacogo: import section: %w", err)
		}
		kind := typeRefToSort(item.Import.Type)
		impDesc := ImportDesc{
			Name: item.Import.Name.Name,
			Kind: kind,
		}
		l.imports = append(l.imports, impDesc)
		switch kind {
		case SortFunc:
			tr, ok := item.Import.Type.(wasmparser.TypeRefFunc)
			if !ok {
				return fmt.Errorf("wacogo: import %q: kind=func but type ref is %T", item.Import.Name.Name, item.Import.Type)
			}
			l.plan = append(l.plan, &planImportFunc{
				name:       item.Import.Name.Name,
				funcTypeID: tr.Index,
			})
			l.funcIsImport = append(l.funcIsImport, true)
		case SortInstance:
			l.plan = append(l.plan, &planImportInstance{
				name:         item.Import.Name.Name,
				runtimeEmpty: item.IsInstanceRuntimeEmpty(),
			})
			l.scope.componentInstances = append(l.scope.componentInstances, loadInstanceInfo{
				importName: item.Import.Name.Name,
			})
		case SortCoreModule:
			idx := uint32(len(l.scope.compiledModules))
			l.scope.compiledModules = append(l.scope.compiledModules, CompiledModule{})
			l.plan = append(l.plan, &planImportModule{
				name:  item.Import.Name.Name,
				index: idx,
			})
		case SortComponent:
			idx := uint32(len(l.scope.subComponents))
			l.scope.subComponents = append(l.scope.subComponents, nil)
			l.plan = append(l.plan, &planImportComponent{
				name:  item.Import.Name.Name,
				index: idx,
			})
		case SortType:
			// Direct component-level type imports ((import "T" (type ...)))
			// resolve at instantiate time by reading the TypeArg from the
			// instantiation args bag.
			l.allocType(importResolver{importName: item.Import.Name.Name})
		}
	}
	return nil
}

// buildComponent assembles the final Component from the accumulated loader state.
func (l *componentLoader) buildComponent(parentScope *loadScope) (*Component, error) {
	var outerModuleClosures map[int]outerRef
	if len(l.scope.outerModuleClosures) > 0 {
		outerModuleClosures = l.scope.outerModuleClosures
	}
	var outerComponentClosures map[int]outerRef
	if len(l.scope.outerComponentClosures) > 0 {
		outerComponentClosures = l.scope.outerComponentClosures
	}

	comp := &Component{
		engine:                 l.engine,
		compiledModules:        l.scope.compiledModules,
		subComponents:          l.scope.subComponents,
		plan:                   l.plan,
		exports:                l.exports,
		imports:                l.imports,
		typeResolvers:          l.typeResolvers,
		outerModuleClosures:    outerModuleClosures,
		outerComponentClosures: outerComponentClosures,
	}

	if parentScope == nil {
		for _, imp := range l.imports {
			if imp.Kind == SortComponent {
				return nil, fmt.Errorf("wacogo: root-level component imports are not supported")
			}
		}
		for _, exp := range l.exports {
			if exp.Kind == SortComponent {
				return nil, fmt.Errorf("wacogo: exporting a component from the root component is not supported")
			}
		}
	}

	return comp, nil
}

// convertComponentExports converts wasmparser ComponentExport to componentExportDesc.
func convertComponentExports(exports []wasmparser.ComponentExport) []componentExportDesc {
	result := make([]componentExportDesc, len(exports))
	for i, exp := range exports {
		result[i] = componentExportDesc{
			name:  exp.Name.Name,
			kind:  componentExternalKindToSort(exp.Kind),
			index: exp.Index,
		}
	}
	return result
}

// mapCanonicalOptions converts wasmparser canonical options to canonicalOptions.
func mapCanonicalOptions(opts []wasmparser.CanonicalOption) canonicalOptions {
	var co canonicalOptions
	for _, opt := range opts {
		switch o := opt.(type) {
		case wasmparser.CanonOptUTF8:
			co.stringEncoding = canon.EncUTF8
		case wasmparser.CanonOptUTF16:
			co.stringEncoding = canon.EncUTF16
		case wasmparser.CanonOptLatin1UTF16:
			co.stringEncoding = canon.EncLatin1UTF16
		case wasmparser.CanonOptMemory:
			co.memory = o.Index
			co.hasMemory = true
		case wasmparser.CanonOptRealloc:
			co.realloc = o.Index
			co.hasRealloc = true
		case wasmparser.CanonOptPostReturn:
			co.postReturn = o.Index
			co.hasPostReturn = true
		}
	}
	return co
}

// componentExternalKindToSort maps a wasmparser ComponentExternalKind to a Sort.
func componentExternalKindToSort(k wasmparser.ComponentExternalKind) Sort {
	switch k {
	case wasmparser.ExternalKindFunc:
		return SortFunc
	case wasmparser.ExternalKindModule:
		return SortCoreModule
	case wasmparser.ExternalKindValue:
		return SortValue
	case wasmparser.ExternalKindType:
		return SortType
	case wasmparser.ExternalKindComponent:
		return SortComponent
	case wasmparser.ExternalKindInstance:
		return SortInstance
	default:
		panic(fmt.Sprintf("wacogo: unknown ComponentExternalKind %d", k))
	}
}

// coreSortToSort maps a wasmparser CoreSort to a Sort.
func coreSortToSort(k wasmparser.CoreSort) Sort {
	switch k {
	case wasmparser.CoreSortFunc:
		return SortCoreFunc
	case wasmparser.CoreSortTable:
		return SortCoreTable
	case wasmparser.CoreSortMemory:
		return SortCoreMemory
	case wasmparser.CoreSortGlobal:
		return SortCoreGlobal
	default:
		panic(fmt.Sprintf("wacogo: unknown CoreSort %d", k))
	}
}

// outerAliasKindToSort maps a wasmparser ComponentOuterAliasKind to a Sort.
func outerAliasKindToSort(k wasmparser.ComponentOuterAliasKind) Sort {
	switch k {
	case wasmparser.OuterAliasKindCoreModule:
		return SortCoreModule
	case wasmparser.OuterAliasKindType:
		return SortType
	case wasmparser.OuterAliasKindComponent:
		return SortComponent
	default:
		panic(fmt.Sprintf("wacogo: outerAliasKindToSort: unhandled alias kind %d", k))
	}
}

// typeRefToSort maps a wasmparser ComponentTypeRef to a Sort.
func typeRefToSort(tr wasmparser.ComponentTypeRef) Sort {
	switch tr.(type) {
	case wasmparser.TypeRefModule:
		return SortCoreModule
	case wasmparser.TypeRefFunc:
		return SortFunc
	case wasmparser.TypeRefValue:
		return SortValue
	case wasmparser.TypeRefType:
		return SortType
	case wasmparser.TypeRefComponent:
		return SortComponent
	case wasmparser.TypeRefInstance:
		return SortInstance
	default:
		panic(fmt.Sprintf("wacogo: unknown ComponentTypeRef %T", tr))
	}
}
