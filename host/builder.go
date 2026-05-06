package host

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/core"
)

// Builder accumulates declarations for a host component. Register
// function, type, and resource exports via AddFunction, AddType,
// AddResource, and AddResourceRef, then call Build to produce a
// reusable Component.
//
// A Builder is not safe for concurrent use.
type Builder struct {
	engine *core.Engine
	name   string

	root *scope

	nextComponentResourceTypeID int
}

type funcDecl struct {
	name string
	ty   *FuncType
	fn   Func
}

type typeDecl struct {
	name string
	expr TypeExpr
	ref  *TypeRef
}

type resourceDecl struct {
	name string
	rt   *ResourceType

	// populated by Build:
	hostModExport string // synthetic dtor export name; "" if no dtor
}

type resourceRefDecl struct {
	name string
	ref  *ResourceTypeRef
}

// NewBuilder returns a fresh Builder for declaring a host component
// against engine. name is used in error messages and synthesized
// wasm-module names.
//
// Most callers should construct a Builder via
// (*wacogo.Engine).NewHostBuilder rather than calling NewBuilder
// directly.
func NewBuilder(e *core.Engine, name string) *Builder {
	return &Builder{engine: e, name: name, root: &scope{}}
}

// AddFunction declares a function export named exportName with the
// signature ty, implemented by fn. Duplicate exportName values are
// rejected by Build.
func (b *Builder) AddFunction(exportName string, ty *FuncType, fn Func) {
	b.root.funcs = append(b.root.funcs, funcDecl{name: exportName, ty: ty, fn: fn})
}

// AddResource declares a host-owned resource type exported as
// exportName. The returned ResourceType produces own<R> and borrow<R>
// TypeExprs via its Own and Borrow methods.
//
// If dtor is non-nil it runs when the last own handle for this
// resource is dropped. Pass nil if no cleanup is required.
//
// exportName must be unique among the Builder's resource exports.
func (b *Builder) AddResource(exportName string, dtor ResourceDtor) *ResourceType {
	rt := &ResourceType{
		builder:                 b,
		exportName:              exportName,
		dtor:                    dtor,
		componentResourceTypeID: b.nextComponentResourceTypeID,
	}
	b.nextComponentResourceTypeID++
	b.root.resources = append(b.root.resources, resourceDecl{name: exportName, rt: rt})
	return rt
}

// AddResourceRef declares a resource type whose identity will be
// supplied by another component instance at Instantiate time via
// WithResourceFrom. The returned ResourceTypeRef produces own<R> and
// borrow<R> TypeExprs via its Own and Borrow methods.
//
// exportName must be unique among the Builder's resource exports.
func (b *Builder) AddResourceRef(exportName string) *ResourceTypeRef {
	r := &ResourceTypeRef{
		builder:                 b,
		exportName:              exportName,
		componentResourceTypeID: b.nextComponentResourceTypeID,
	}
	b.nextComponentResourceTypeID++
	b.root.resourceRefs = append(b.root.resourceRefs, resourceRefDecl{name: exportName, ref: r})
	return r
}

// AddType registers typ and returns a TypeRef usable wherever a
// TypeExpr is accepted. exportName is the name typ is exported
// under; pass "" to share a structural type internally without
// exporting it.
//
// Nominal kinds (Record, Variant, Flags, Enum) must be registered
// with a non-empty exportName and cannot be used inline. Structural
// kinds (List, Tuple, Option, Result) may be registered or used
// inline. Primitives and own/borrow handles must not be registered.
// Violations surface as Build errors.
func (b *Builder) AddType(exportName string, typ TypeExpr) *TypeRef {
	ref := &TypeRef{declExpr: typ}
	b.root.types = append(b.root.types, typeDecl{name: exportName, expr: typ, ref: ref})
	return ref
}

// AddResourceAlias re-exports rt under an additional name. Both
// exports share the same resource identity at runtime and the same
// ResourceID in the component-instance type. rt must come from this
// Builder.
//
// Aliasing a *ResourceTypeRef is not supported.
func (b *Builder) AddResourceAlias(name string, rt *ResourceType) error {
	if rt == nil {
		return fmt.Errorf("wacogo/host: AddResourceAlias %q: nil *ResourceType", name)
	}
	if rt.builder != b {
		return fmt.Errorf("wacogo/host: AddResourceAlias %q: target *ResourceType belongs to a different Builder", name)
	}
	b.root.aliases = append(b.root.aliases, aliasDecl{name: name, rt: rt})
	return nil
}

// AddNestedInstance declares a nested-instance export at name. The
// returned *InstanceBuilder accepts the same declarations as the
// top-level Builder, scoped to that instance. Recursion is supported.
func (b *Builder) AddNestedInstance(name string) *InstanceBuilder {
	child := &scope{parent: b.root, name: name}
	b.root.nested = append(b.root.nested, child)
	return &InstanceBuilder{parent: b, scope: child}
}

// AddCoreModule registers a precompiled core wasm module exported
// under name. The wasm bytes are compiled immediately against the
// engine's wazero runtime, and the module's exports are extracted
// for the component-level type advertisement; a compile or parse
// error is returned synchronously.
//
// The same compiled module is shared across all Instantiate calls.
func (b *Builder) AddCoreModule(name string, wasmBytes []byte) error {
	rt := core.WazeroRuntime(b.engine)
	cm, err := rt.CompileModule(context.Background(), wasmBytes)
	if err != nil {
		return fmt.Errorf("wacogo/host: AddCoreModule %q: %w", name, err)
	}
	td, err := core.Validator(b.engine).CoreModuleTypeFromBytes(wasmBytes)
	if err != nil {
		return fmt.Errorf("wacogo/host: AddCoreModule %q: extract type: %w", name, err)
	}
	b.root.coreModules = append(b.root.coreModules, coreModuleDecl{
		name:     name,
		compiled: core.WrapCompiledModule(cm),
		typeDesc: td,
	})
	return nil
}

// Build validates the accumulated declarations and returns a
// Component template ready for Instantiate. The Builder must not be
// used further after Build returns successfully.
//
// Build reports the first problem encountered: duplicate export
// names, nominal compounds used inline, unresolved TypeRefs, or
// invalid signatures.
func (b *Builder) Build(ctx context.Context) (*Component, error) {
	comp := &Component{engine: b.engine, name: b.name}
	if err := buildScopeTree(b, comp, b.root); err != nil {
		return nil, err
	}
	cm, err := compileStubModule(ctx, core.WazeroRuntime(b.engine), comp)
	if err != nil {
		return nil, err
	}
	comp.compiledStub = cm
	return comp, nil
}

// buildScopeTree validates names and assigns runtime IDs across the
// whole tree, populating comp.allFuncs, comp.allResources, and the
// parallel comp.root *scopeRuntime tree.
func buildScopeTree(b *Builder, comp *Component, root *scope) error {
	var visit func(s *scope, scopePath string) (*scopeRuntime, error)
	visit = func(s *scope, scopePath string) (*scopeRuntime, error) {
		seen := map[string]bool{}
		check := func(name string) error {
			if seen[name] {
				return fmt.Errorf("wacogo/host: duplicate export %q in instance %q", name, s.name)
			}
			seen[name] = true
			return nil
		}
		sr := &scopeRuntime{exportName: s.name}
		for i := range s.funcs {
			fd := &s.funcs[i]
			if err := check(fd.name); err != nil {
				return nil, err
			}
			if err := validateFuncType(fd.ty); err != nil {
				return nil, fmt.Errorf("wacogo/host: func %q: %w", fd.name, err)
			}
			flatParams, flatResults, err := flattenFuncForStub(fd.ty)
			if err != nil {
				return nil, fmt.Errorf("wacogo/host: func %q flat: %w", fd.name, err)
			}
			fr := &funcRuntime{
				exportName:     fd.name,
				stubExportName: stubExportNameOf(len(comp.allFuncs), scopePath, fd.name),
				userFn:         fd.fn,
				userFT:         fd.ty,
				flatParams:     flatParams,
				flatResults:    flatResults,
			}
			fr.hostModExport = fr.stubExportName
			comp.allFuncs = append(comp.allFuncs, fr)
			sr.funcs = append(sr.funcs, fr)
		}
		for i := range s.resources {
			rd := &s.resources[i]
			if err := check(rd.name); err != nil {
				return nil, err
			}
			rr := &resourceRuntime{
				exportName: rd.name,
				userDtor:   rd.rt.dtor,
				rt:         rd.rt,
			}
			if rd.rt.dtor != nil {
				rr.hostModExport = dtorExportNameOf(len(comp.allResources), scopePath, rd.name)
				rd.hostModExport = rr.hostModExport
			}
			comp.allResources = append(comp.allResources, rr)
			sr.resources = append(sr.resources, rr)
		}
		for i := range s.resourceRefs {
			rrd := &s.resourceRefs[i]
			if err := check(rrd.name); err != nil {
				return nil, err
			}
			sr.resourceRefs = append(sr.resourceRefs, *rrd)
		}
		for i := range s.aliases {
			ad := &s.aliases[i]
			if err := check(ad.name); err != nil {
				return nil, err
			}
			rr := findResourceRuntime(comp, ad.rt)
			if rr == nil {
				return nil, fmt.Errorf("wacogo/host: AddResourceAlias %q: target *ResourceType is not registered with this Builder", ad.name)
			}
			sr.aliases = append(sr.aliases, aliasRuntime{exportName: ad.name, target: rr})
		}
		for i := range s.coreModules {
			cmd := &s.coreModules[i]
			if err := check(cmd.name); err != nil {
				return nil, err
			}
			sr.coreModules = append(sr.coreModules, coreModuleRuntime{
				exportName: cmd.name,
				compiled:   cmd.compiled,
				typeDesc:   cmd.typeDesc,
			})
		}
		for _, ts := range s.types {
			if err := validateTopLevelTypeExpr(ts.expr); err != nil {
				return nil, fmt.Errorf("wacogo/host: AddType %q: %w", ts.name, err)
			}
		}
		for _, child := range s.nested {
			if err := check(child.name); err != nil {
				return nil, err
			}
			childPath := scopePath
			if childPath != "" {
				childPath += "/"
			}
			childPath += child.name
			childRT, err := visit(child, childPath)
			if err != nil {
				return nil, err
			}
			sr.nested = append(sr.nested, childRT)
		}
		return sr, nil
	}
	rootRT, err := visit(root, "")
	if err != nil {
		return err
	}
	comp.root = rootRT
	// Populate comp.resourceRefs from the root scope to preserve the
	// pre-refactor field used by Component.Instantiate's resource-ref
	// resolver. Only root-scope ResourceTypeRefs are supported in this
	// pass; nested scopes have no refs yet.
	comp.resourceRefs = root.resourceRefs
	return nil
}

// stubExportNameOf builds the synthetic stub-level export name for a
// host func. The name embeds the flat func index, the lexical scope
// path, and the user-facing export name so wazero stack traces and
// runtime error messages identify the func unambiguously even when
// nested-instance scopes share user-facing names.
func stubExportNameOf(flatIdx int, scopePath, exportName string) string {
	return fmt.Sprintf("__fn_%d_%s", flatIdx, sanitizeStubName(joinScopeName(scopePath, exportName)))
}

// dtorExportNameOf is the matching helper for resource dtors.
func dtorExportNameOf(flatIdx int, scopePath, exportName string) string {
	return fmt.Sprintf("__dtor_%d_%s", flatIdx, sanitizeStubName(joinScopeName(scopePath, exportName)))
}

func joinScopeName(scopePath, name string) string {
	if scopePath == "" {
		return name
	}
	return scopePath + "/" + name
}

// sanitizeStubName replaces characters outside [A-Za-z0-9_] with '_'.
// Empty inputs yield "_". Wasm permits arbitrary UTF-8 in export names,
// but the engine prefers ASCII-only identifiers in stack traces.
func sanitizeStubName(s string) string {
	if s == "" {
		return "_"
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// findResourceRuntime returns the resourceRuntime that wraps rt, or
// nil if rt is not part of this Component (i.e., was created against
// a different Builder).
func findResourceRuntime(comp *Component, rt *ResourceType) *resourceRuntime {
	for _, rr := range comp.allResources {
		if rr.rt == rt {
			return rr
		}
	}
	return nil
}
