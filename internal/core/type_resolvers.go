package core

import (
	"context"
	"fmt"
)

// typeResolver constructs a Type value for one slot in a component's type
// index space. Each TypeID slot has exactly one resolver, run once per
// ComponentInstance to populate ComponentInstance.types[typeID].
type typeResolver interface {
	resolve(rc *resolverCtx) Type
}

// resolverCtx is the per-run context passed to typeResolver.resolve. Stack
// allocated by planResolveType.execute; discarded at return. Holds only the
// state resolvers actually need — the instance whose type table is being
// built (reaches its component, parent, and sibling instances) plus the
// instantiation args bag (used by importResolver for direct type imports)
// and the in-progress instantiation state (used by resourceResolver to
// resolve a dtor core func against the component's index spaces).
type resolverCtx struct {
	inst    *ComponentInstance
	imports map[string]any
	state   *instantiationState
}

// primitiveResolver holds a pre-constructed singleton Type value and returns
// it unchanged. Cost is a field read; no allocation at resolve time.
type primitiveResolver struct {
	t Type
}

func (r primitiveResolver) resolve(*resolverCtx) Type { return r.t }

// indexResolver reads a Type by index from the current instance's type table.
// Used as a child resolver within compound resolvers to reference another
// TypeID slot in the component's type index space.
type indexResolver struct{ idx uint32 }

func (r indexResolver) resolve(rc *resolverCtx) Type { return rc.inst.types[r.idx] }

type listResolver struct{ elem typeResolver }

func (r listResolver) resolve(rc *resolverCtx) Type {
	return TypeList{Elem: r.elem.resolve(rc)}
}

type recordResolverField struct {
	name string
	t    typeResolver
}
type recordResolver struct{ fields []recordResolverField }

func (r recordResolver) resolve(rc *resolverCtx) Type {
	out := make([]FieldType, len(r.fields))
	for i, f := range r.fields {
		out[i] = FieldType{Name: f.name, Type: f.t.resolve(rc)}
	}
	return TypeRecord{Fields: out}
}

type tupleResolver struct{ elems []typeResolver }

func (r tupleResolver) resolve(rc *resolverCtx) Type {
	out := make([]Type, len(r.elems))
	for i, ref := range r.elems {
		out[i] = ref.resolve(rc)
	}
	return TypeTuple{Types: out}
}

type variantResolverCase struct {
	name       string
	payload    typeResolver
	hasPayload bool
}
type variantResolver struct{ cases []variantResolverCase }

func (r variantResolver) resolve(rc *resolverCtx) Type {
	out := make([]CaseType, len(r.cases))
	for i, c := range r.cases {
		out[i].Name = c.name
		if c.hasPayload {
			out[i].Payload = c.payload.resolve(rc)
		}
	}
	return TypeVariant{Cases: out}
}

type enumResolver struct{ cases []string }

func (r enumResolver) resolve(*resolverCtx) Type { return TypeEnum{Cases: r.cases} }

type optionResolver struct{ inner typeResolver }

func (r optionResolver) resolve(rc *resolverCtx) Type {
	return TypeOption{Inner: r.inner.resolve(rc)}
}

type resultResolver struct {
	ok, err       typeResolver
	hasOk, hasErr bool
}

func (r resultResolver) resolve(rc *resolverCtx) Type {
	var ok, err Type
	if r.hasOk {
		ok = r.ok.resolve(rc)
	}
	if r.hasErr {
		err = r.err.resolve(rc)
	}
	return TypeResult{Ok: ok, Err: err}
}

type flagsResolver struct{ names []string }

func (r flagsResolver) resolve(*resolverCtx) Type { return TypeFlags{Names: r.names} }

type funcResolverParam struct {
	name string
	t    typeResolver
}
type funcResolver struct {
	params, results []funcResolverParam
}

func (r funcResolver) resolve(rc *resolverCtx) Type {
	params := make([]ParamType, len(r.params))
	for i, p := range r.params {
		t := p.t.resolve(rc)
		if t == nil {
			// Child type slot is unresolved (e.g. a direct component-level type
			// import, which is not currently plumbed through the WithImport
			// carrier mechanism). Return nil so the caller can fall back to the
			// pre-typeResolvers behavior for functions with unresolvable
			// signatures.
			return nil
		}
		params[i] = ParamType{Name: p.name, Type: t}
	}
	results := make([]ResultType, len(r.results))
	for i, p := range r.results {
		t := p.t.resolve(rc)
		if t == nil {
			return nil
		}
		results[i] = ResultType{Name: p.name, Type: t}
	}
	return &FuncType{Params: params, Results: results}
}

type ownResolver struct{ resource uint32 }

func (r ownResolver) resolve(rc *resolverCtx) Type {
	t := rc.inst.types[r.resource]
	if t == nil {
		// Referenced resource type is unresolved; return nil so parent
		// structural resolvers can fall back to nil.
		return nil
	}
	return TypeOwn{ResourceType: t.(*TypeResource)}
}

type borrowResolver struct{ resource uint32 }

func (r borrowResolver) resolve(rc *resolverCtx) Type {
	t := rc.inst.types[r.resource]
	if t == nil {
		return nil
	}
	return TypeBorrow{ResourceType: t.(*TypeResource)}
}

// resourceResolver mints a fresh *TypeResource per resolve call — the only
// intentionally non-pure resolver kind. Per-instantiation identity is the
// whole point: two instantiations of the same component produce two distinct
// *TypeResource values that compare unequal.
//
// hasDtor / dtorFuncIdx describe the dtor as a core function index in the
// component's core-function index space. Resolution against the live
// instance happens inline during planResolveType execution.
type resourceResolver struct {
	hasDtor     bool
	dtorFuncIdx uint32
}

func (r resourceResolver) resolve(rc *resolverCtx) Type {
	rt := &TypeResource{instance: rc.inst}
	if r.hasDtor && rc.state != nil {
		if dtor := rc.state.resolveDtor(r.dtorFuncIdx); dtor != nil {
			// Wrap the wasm-callable api.Function so canon's
			// ResourceTable.Drop can invoke it via the
			// ResourceType.Destructor() path. Captures dtor by
			// value; safe to retain across the instance lifetime.
			rt.dtor = func(ctx context.Context, rep uint32) error {
				_, err := dtor.Call(ctx, uint64(rep))
				return err
			}
		}
	}
	return rt
}

type instanceImportResolver struct {
	instanceIdx uint32
	exportName  string
}

func (r instanceImportResolver) resolve(rc *resolverCtx) Type {
	if int(r.instanceIdx) >= len(rc.inst.instances) {
		// Instance index space is out of range — this is a validator-guaranteed
		// invariant so reaching this is a loader/validator bug.
		panic(fmt.Sprintf("wacogo: instanceImportResolver: instance %d out of range (have %d)", r.instanceIdx, len(rc.inst.instances)))
	}
	inst := rc.inst.instances[r.instanceIdx]
	if inst == nil {
		// Instance slot present but not yet populated (import not satisfied).
		// Return nil so callers can surface an instantiation-time error.
		return nil
	}
	return inst.exportedType(r.exportName)
}

type aliasResolver struct {
	outerDepth uint32
	typeIdx    uint32
}

func (r aliasResolver) resolve(rc *resolverCtx) Type {
	target := rc.inst
	for range r.outerDepth {
		if target.parent == nil {
			panic("wacogo: aliasResolver: outerDepth exceeds nesting")
		}
		target = target.parent
	}
	return target.types[r.typeIdx]
}

// importResolver reads a Type from the instantiation args bag for a direct
// component-level type import. Returns nil if the import was not supplied or
// the supplied value isn't a Type — planResolveType surfaces that as a
// regular instantiation error, not a panic.
type importResolver struct{ importName string }

func (r importResolver) resolve(rc *resolverCtx) Type {
	arg, ok := rc.imports[r.importName]
	if !ok {
		return nil
	}
	t, _ := arg.(Type)
	return t
}

type instanceTypeResolver struct{}

func (instanceTypeResolver) resolve(*resolverCtx) Type { return &InstanceType{} }

type componentTypeResolver struct{}

func (componentTypeResolver) resolve(*resolverCtx) Type { return &ComponentType{} }
