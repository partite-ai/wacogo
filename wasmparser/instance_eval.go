package wasmparser

import "slices"

// componentBody records how a component fills its instance and component index
// spaces, in order. It is what makes an instance's type, and those of the
// instances inside it, a function of the component's type and what is supplied
// to its imports.
type componentBody struct {
	steps []bodyStep
}

// bodyStep appends one entry to an instance or component index space.
type bodyStep interface{ isBodyStep() }

type stepImportInstance struct {
	name string
	view ComponentInstanceTypeID
}

type stepImportComponent struct{ name string }

type stepAliasInstance struct {
	from uint32
	name string
	view ComponentInstanceTypeID
}

type stepAliasComponent struct {
	from uint32
	name string
}

type stepOuterComponent struct{ count, index uint32 }

type stepDefineComponent struct{ id ComponentTypeID }

type stepExportInstance struct {
	name string
	from uint32
}

type stepExportComponent struct {
	name string
	from uint32
}

// bodyRef names an index-space entry supplied to an instantiation or an inline
// instance.
type bodyRef struct {
	name  string
	kind  ComponentExternalKind
	index uint32
}

type stepInstantiate struct {
	component uint32
	args      []bodyRef
	// resources holds the resource types among args, which are IDs rather than
	// index-space entries once resolved.
	resources map[string]ResourceID
	view      ComponentInstanceTypeID
}

type stepInlineInstance struct {
	exports []bodyRef
	view    ComponentInstanceTypeID
}

func (stepImportInstance) isBodyStep()  {}
func (stepImportComponent) isBodyStep() {}
func (stepAliasInstance) isBodyStep()   {}
func (stepAliasComponent) isBodyStep()  {}
func (stepOuterComponent) isBodyStep()  {}
func (stepDefineComponent) isBodyStep() {}
func (stepExportInstance) isBodyStep()  {}
func (stepExportComponent) isBodyStep() {}
func (stepInstantiate) isBodyStep()     {}
func (stepInlineInstance) isBodyStep()  {}

// record appends step to the body of the component being validated. Type
// declarations have no body.
func (cs *ComponentState) record(step bodyStep) {
	if cs.kind == ComponentKindComponent {
		cs.body.steps = append(cs.body.steps, step)
	}
}

// componentValue is a component held in an index space: its type, and the frame
// it was defined in, which its outer aliases refer to.
type componentValue struct {
	id    ComponentTypeID
	frame *instanceFrame
}

// instanceArgs is what an instantiation supplies to a component's imports.
type instanceArgs struct {
	instances  map[string]*InstanceType
	components map[string]*componentValue
	resources  map[string]scopedResID
}

// instance returns the instance supplied for an import, under its literal or
// canonical name.
func (a instanceArgs) instance(name string) *InstanceType {
	if inst, ok := a.instances[name]; ok {
		return inst
	}
	return a.instances[CanonicalizeImportName(name)]
}

// instanceFrame holds the index spaces of one instantiation being evaluated.
type instanceFrame struct {
	self       *InstanceType
	outer      *instanceFrame
	args       instanceArgs
	instances  []*InstanceType
	components []*componentValue
}

// evalInstance returns the type of an instantiation of component id. outer is
// the frame the component was defined in.
//
// Evaluation order is what lets resourceRoots be resolved as they are recorded:
// an instantiation's imports refer to instances evaluated before it, and its
// parent binds to it only once it, and everything inside it, is complete.
func evalInstance(arena *TypeArena, id ComponentTypeID, outer *instanceFrame, args instanceArgs) *InstanceType {
	ct := arena.ComponentTypes[id]
	self := &InstanceType{arena: arena, exports: ct.Exports}
	self.scope = self
	self.bindImports(ct, args)
	if ct.body != nil {
		f := &instanceFrame{self: self, outer: outer, args: args}
		for _, step := range ct.body.steps {
			f.run(step)
		}
		self.instances = f.instances
	}
	return self
}

// bindImports records the roots of the resources this instance's imports define.
func (i *InstanceType) bindImports(ct ComponentTypeDesc, args instanceArgs) {
	for _, name := range ct.ImportOrder {
		imp := ct.Imports[name]
		switch {
		case imp.Kind == EntityInstance:
			if lender := args.instance(name); lender != nil {
				i.bindImportedInstance(imp.InstID, lender, nil)
			}
		case imp.Kind == EntityType && imp.TypeRef.Kind == AnyTypeResource:
			if root, ok := args.resources[name]; ok {
				i.setRoot(i.arena.ResolveResourceAlias(imp.TypeRef.ResID), root)
			}
		}
	}
}

func (i *InstanceType) bindImportedInstance(id ComponentInstanceTypeID, lender *InstanceType, path []string) {
	it := i.arena.InstanceTypes[id]
	for _, res := range it.DefinedResources {
		if lenderResID, ok := lender.resourceAt(slices.Concat(path, []string{res.Export})); ok {
			i.setRoot(i.arena.ResolveResourceAlias(res.ID), resolveResource(lender.scope, lender.arena, lenderResID, nil))
		}
	}
	for name, et := range it.Exports {
		if et.Kind == EntityInstance {
			i.bindImportedInstance(et.InstID, lender, slices.Concat(path, []string{name}))
		}
	}
}

func (i *InstanceType) setRoot(id ResourceID, root scopedResID) {
	if i.scope.resourceRoots == nil {
		i.scope.resourceRoots = make(map[ResourceID]scopedResID)
	}
	i.scope.resourceRoots[id] = root
}

// bindSubInstance records that the resources in this instantiation's view of a
// sub-instance are those of the sub-instance's actual exports. The two have to be
// matched by path: the view comes from the type the component was declared with,
// which for an imported component is not the type of the component supplied.
func (i *InstanceType) bindSubInstance(view map[string]ComponentEntityType, child *InstanceType, actual map[string]ComponentEntityType) {
	for name, v := range view {
		a, ok := actual[name]
		if !ok || a.Kind != v.Kind {
			continue
		}
		switch {
		case v.Kind == EntityInstance:
			i.bindSubInstance(i.arena.InstanceTypes[v.InstID].Exports, child, child.arena.InstanceTypes[a.InstID].Exports)
		case v.Kind == EntityType && v.TypeRef.Kind == AnyTypeResource && a.TypeRef.Kind == AnyTypeResource:
			id := i.arena.ResolveResourceAlias(v.TypeRef.ResID)
			if _, bound := i.scope.resourceRoots[id]; !bound {
				i.setRoot(id, resolveResource(child.scope, child.arena, a.TypeRef.ResID, nil))
			}
		}
	}
}

// declared returns an instance of the given declared type, sharing this frame's
// resource identities. It stands for an instance assembled here, or for one that
// was omitted because it needs nothing at runtime.
func (f *instanceFrame) declared(view ComponentInstanceTypeID) *InstanceType {
	arena := f.self.arena
	return &InstanceType{arena: arena, exports: arena.InstanceTypes[view].Exports, scope: f.self.scope}
}

func (f *instanceFrame) run(step bodyStep) {
	switch s := step.(type) {
	case stepImportInstance:
		inst := f.args.instance(s.name)
		if inst == nil {
			inst = f.declared(s.view)
		}
		f.instances = append(f.instances, inst)
	case stepImportComponent:
		f.components = append(f.components, f.args.components[s.name])
	case stepAliasInstance:
		inst := f.instances[s.from].ExportedInstance(s.name)
		if inst == nil {
			inst = f.declared(s.view)
		}
		f.instances = append(f.instances, inst)
	case stepAliasComponent:
		f.components = append(f.components, f.instances[s.from].components[s.name])
	case stepOuterComponent:
		target := f
		for range s.count {
			target = target.outer
		}
		f.components = append(f.components, target.components[s.index])
	case stepDefineComponent:
		f.components = append(f.components, &componentValue{id: s.id, frame: f})
	case stepExportInstance:
		f.self.setChild(s.name, f.instances[s.from])
		f.instances = append(f.instances, f.instances[s.from])
	case stepExportComponent:
		f.self.setComponent(s.name, f.components[s.from])
		f.components = append(f.components, f.components[s.from])
	case stepInstantiate:
		f.instances = append(f.instances, f.instantiate(s))
	case stepInlineInstance:
		inst := f.declared(s.view)
		for _, exp := range s.exports {
			switch exp.kind {
			case ExternalKindInstance:
				inst.setChild(exp.name, f.instances[exp.index])
			case ExternalKindComponent:
				inst.setComponent(exp.name, f.components[exp.index])
			}
		}
		f.instances = append(f.instances, inst)
	}
}

func (f *instanceFrame) instantiate(s stepInstantiate) *InstanceType {
	comp := f.components[s.component]
	if comp == nil {
		// Its source was supplied by Go without a type, or not at all. Linking
		// reports that; until then all that's known is the declared type.
		return f.declared(s.view)
	}
	args := instanceArgs{
		instances:  make(map[string]*InstanceType),
		components: make(map[string]*componentValue),
		resources:  make(map[string]scopedResID, len(s.resources)),
	}
	for _, arg := range s.args {
		switch arg.kind {
		case ExternalKindInstance:
			args.instances[arg.name] = f.instances[arg.index]
		case ExternalKindComponent:
			args.components[arg.name] = f.components[arg.index]
		}
	}
	self := f.self
	for name, id := range s.resources {
		args.resources[name] = resolveResource(self.scope, self.arena, id, nil)
	}
	child := evalInstance(self.arena, comp.id, comp.frame, args)
	self.bindSubInstance(self.arena.InstanceTypes[s.view].Exports, child, child.exports)
	return child
}
