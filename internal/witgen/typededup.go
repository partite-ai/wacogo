package witgen

import "sort"

// CollectTypes returns the unique types appearing anywhere in iface's
// function signatures (recursively into compounds), ordered by
// TypeName for determinism.
func CollectTypes(iface *Interface) []Type {
	seen := map[string]Type{}
	for _, f := range iface.Funcs {
		for _, p := range f.Params {
			walkType(p.Type, seen)
		}
		if f.Result != nil {
			walkType(f.Result, seen)
		}
	}
	// Walk variant case payloads and record field types so anonymous
	// compound types embedded in nominal declarations (e.g. the
	// tuple<u8,u8,u8,u8> inside ip-address.ipv4) get collected and have
	// their Go declarations emitted.
	for _, v := range iface.Variants {
		for _, c := range v.Cases {
			if c.Payload != nil {
				walkType(c.Payload, seen)
			}
		}
	}
	for _, r := range iface.Records {
		for _, f := range r.Fields {
			walkType(f.Type, seen)
		}
	}
	// Also walk resource method/ctor/static signatures directly so
	// compound types appearing only in resource signatures (e.g.
	// result<list<u8>, stream-error> on InputStream.Read) get collected.
	for _, r := range iface.Resources {
		if r.Ctor != nil {
			for _, p := range r.Ctor.Params {
				walkType(p.Type, seen)
			}
		}
		for _, m := range r.Methods {
			for _, p := range m.Params {
				walkType(p.Type, seen)
			}
			if m.Result != nil {
				walkType(m.Result, seen)
			}
		}
		for _, s := range r.Statics {
			for _, p := range s.Params {
				walkType(p.Type, seen)
			}
			if s.Result != nil {
				walkType(s.Result, seen)
			}
		}
	}
	// Walk imported resources' qualified method signatures so types
	// referenced by the importer-emitted forwarder bodies (which call
	// liftFlat<X> / liftMem<X> / lowerFlat<X> / lowerMem<X>) get their
	// helper definitions emitted in this package.
	for _, imp := range iface.Imports {
		for _, ir := range imp.Resources {
			methods := ir.QualifiedMethods
			if methods == nil {
				continue
			}
			for _, m := range methods {
				for _, p := range m.Params {
					walkType(p.Type, seen)
				}
				if m.Result != nil {
					walkType(m.Result, seen)
				}
			}
		}
	}

	out := make([]Type, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return TypeName(out[i]) < TypeName(out[j])
	})
	return out
}

// walkType recurses into compound types, adding every encountered type
// to seen (keyed by TypeName, which is structurally unique).
func walkType(t Type, seen map[string]Type) {
	if t == nil {
		return
	}
	name := TypeName(t)
	if _, exists := seen[name]; exists {
		return
	}
	seen[name] = t
	switch v := t.(type) {
	case *TypeList:
		walkType(v.Elem, seen)
	case *TypeTuple:
		for _, f := range v.Fields {
			walkType(f, seen)
		}
	case *TypeOption:
		walkType(v.Elem, seen)
	case *TypeResult:
		walkType(v.OK, seen) // walkType already handles nil
		walkType(v.Err, seen)
	case *TypeRecord:
		for _, f := range v.Fields {
			walkType(f.Type, seen)
		}
	case *TypeVariant:
		for _, c := range v.Cases {
			if c.Payload != nil {
				walkType(c.Payload, seen)
			}
		}
	case *TypeResource:
		for _, m := range v.Methods {
			for _, p := range m.Params {
				walkType(p.Type, seen)
			}
			if m.Result != nil {
				walkType(m.Result, seen)
			}
		}
		if v.Ctor != nil {
			for _, p := range v.Ctor.Params {
				walkType(p.Type, seen)
			}
		}
		for _, s := range v.Statics {
			for _, p := range s.Params {
				walkType(p.Type, seen)
			}
			if s.Result != nil {
				walkType(s.Result, seen)
			}
		}
	case *TypeOwn:
		walkType(v.Resource, seen)
	case *TypeBorrow:
		walkType(v.Resource, seen)
	}
}

// TupleTypes returns the subset of types that need a Go struct
// declaration in the current package (only unqualified tuples — qualified
// ones are owned by an imported package and must not be redeclared).
func TupleTypes(types []Type) []*TypeTuple {
	var out []*TypeTuple
	for _, t := range types {
		if tup, ok := t.(*TypeTuple); ok && tup.GoPackageQualifier == "" {
			out = append(out, tup)
		}
	}
	return out
}

// OptionTypes returns the subset of types that are *TypeOption and are
// unqualified (i.e., the struct declaration belongs to this package).
// Qualified options (GoPackageQualifier != "") live in another package and
// only need helper functions, not a local struct declaration.
func OptionTypes(types []Type) []*TypeOption {
	var out []*TypeOption
	for _, t := range types {
		if o, ok := t.(*TypeOption); ok && o.GoPackageQualifier == "" {
			out = append(out, o)
		}
	}
	return out
}

// ResultTypes returns the subset of types that are *TypeResult and
// are unqualified (i.e., the struct declaration belongs to this package).
// Qualified results (GoPackageQualifier != "") live in another package
// and only need helper functions, not a local struct declaration.
func ResultTypes(types []Type) []*TypeResult {
	var out []*TypeResult
	for _, t := range types {
		if r, ok := t.(*TypeResult); ok && r.GoPackageQualifier == "" {
			out = append(out, r)
		}
	}
	return out
}

// RecordTypes returns the subset of types that are *TypeRecord.
func RecordTypes(types []Type) []*TypeRecord {
	var out []*TypeRecord
	for _, t := range types {
		if r, ok := t.(*TypeRecord); ok {
			out = append(out, r)
		}
	}
	return out
}

// HelperTypes returns the subset of types that need lift/lower helper
// functions. Primitives are inlined; enums and flags are inlined as
// typed-uint casts; resource handles are inlined via c.NewOwn / c.Rep —
// so all three are excluded here.
func HelperTypes(types []Type) []Type {
	var out []Type
	for _, t := range types {
		switch t.(type) {
		case Prim, *TypeEnum, *TypeFlags:
			continue // inlined; no helper
		case *TypeResource, *TypeOwn, *TypeBorrow:
			continue // inlined via c.NewOwn / c.Rep
		}
		out = append(out, t)
	}
	return out
}

// VariantTypes returns the subset of types that are *TypeVariant.
func VariantTypes(types []Type) []*TypeVariant {
	var out []*TypeVariant
	for _, t := range types {
		if v, ok := t.(*TypeVariant); ok {
			out = append(out, v)
		}
	}
	return out
}

// ResourceTypes returns the subset of types that are *TypeResource.
func ResourceTypes(types []Type) []*TypeResource {
	var out []*TypeResource
	for _, t := range types {
		if r, ok := t.(*TypeResource); ok {
			out = append(out, r)
		}
	}
	return out
}
