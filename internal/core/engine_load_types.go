package core

import (
	"fmt"

	"github.com/partite-ai/wacogo/wasmparser"
)

// primitiveTypeFor maps a wasmparser primitive kind to the corresponding
// root-package primitive Type singleton. PrimErrorContext has no current
// root-package representation and triggers a panic — error-context support
// is out of scope for the initial ABI.
func primitiveTypeFor(p wasmparser.PrimitiveValType) Type {
	switch p {
	case wasmparser.PrimBool:
		return TypeBool{}
	case wasmparser.PrimS8:
		return TypeS8{}
	case wasmparser.PrimU8:
		return TypeU8{}
	case wasmparser.PrimS16:
		return TypeS16{}
	case wasmparser.PrimU16:
		return TypeU16{}
	case wasmparser.PrimS32:
		return TypeS32{}
	case wasmparser.PrimU32:
		return TypeU32{}
	case wasmparser.PrimS64:
		return TypeS64{}
	case wasmparser.PrimU64:
		return TypeU64{}
	case wasmparser.PrimF32:
		return TypeF32{}
	case wasmparser.PrimF64:
		return TypeF64{}
	case wasmparser.PrimChar:
		return TypeChar{}
	case wasmparser.PrimString:
		return TypeString{}
	}
	panic(fmt.Sprintf("wacogo: unsupported primitive %d", p))
}

// componentValTypeResolver converts a ComponentValType (which is either an
// inline primitive or a reference to another TypeID) into a typeResolver
// that a compound resolver can embed as a child.
func componentValTypeResolver(vt wasmparser.ComponentValType) typeResolver {
	switch v := vt.(type) {
	case wasmparser.PrimitiveValType:
		return primitiveResolver{t: primitiveTypeFor(v)}
	case wasmparser.TypeIndexValType:
		return indexResolver{idx: uint32(v)}
	}
	panic(fmt.Sprintf("wacogo: unhandled ComponentValType %T", vt))
}

// buildDefinedTypeResolver translates a wasmparser.ComponentDefinedType into
// a typeResolver. The validator guarantees the binary is well-typed; this
// function is a pure structural dispatch.
func buildDefinedTypeResolver(dt wasmparser.ComponentDefinedType) typeResolver {
	switch t := dt.(type) {
	case wasmparser.PrimitiveValType:
		return primitiveResolver{t: primitiveTypeFor(t)}
	case wasmparser.TypeIndexValType:
		// A type section entry whose body is just a type-index reference is a
		// type alias — resolve by looking up the target TypeID at resolve time.
		return indexResolver{idx: uint32(t)}
	case *wasmparser.RecordType:
		fields := make([]recordResolverField, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = recordResolverField{name: f.Name, t: componentValTypeResolver(f.Type)}
		}
		return recordResolver{fields: fields}
	case *wasmparser.VariantType:
		cases := make([]variantResolverCase, len(t.Cases))
		for i, c := range t.Cases {
			out := variantResolverCase{name: c.Name}
			if c.Type.Valid {
				out.payload = componentValTypeResolver(c.Type.Value)
				out.hasPayload = true
			}
			cases[i] = out
		}
		return variantResolver{cases: cases}
	case *wasmparser.ListType:
		return listResolver{elem: componentValTypeResolver(t.Element)}
	case *wasmparser.TupleType:
		elems := make([]typeResolver, len(t.Types))
		for i, e := range t.Types {
			elems[i] = componentValTypeResolver(e)
		}
		return tupleResolver{elems: elems}
	case *wasmparser.FlagsType:
		return flagsResolver{names: t.Labels}
	case *wasmparser.EnumType:
		return enumResolver{cases: t.Labels}
	case *wasmparser.OptionType:
		return optionResolver{inner: componentValTypeResolver(t.Inner)}
	case *wasmparser.ResultType:
		out := resultResolver{}
		if t.Ok.Valid {
			out.ok = componentValTypeResolver(t.Ok.Value)
			out.hasOk = true
		}
		if t.Err.Valid {
			out.err = componentValTypeResolver(t.Err.Value)
			out.hasErr = true
		}
		return out
	case *wasmparser.OwnType:
		return ownResolver{resource: t.ResourceIndex}
	case *wasmparser.BorrowType:
		return borrowResolver{resource: t.ResourceIndex}
	}
	panic(fmt.Sprintf("wacogo: unhandled ComponentDefinedType %T", dt))
}

// buildComponentTypeResolver translates a top-level wasmparser.ComponentTypeDef
// (the union over defined types, func types, resource types, and component/
// instance type decls) into a typeResolver. Used by processTypeSection.
func buildComponentTypeResolver(ct wasmparser.ComponentTypeDef) typeResolver {
	switch t := ct.(type) {
	case *wasmparser.ComponentDefinedTypeEntry:
		return buildDefinedTypeResolver(t.Type)
	case *wasmparser.ComponentFuncType:
		params := make([]funcResolverParam, len(t.Params))
		for i, p := range t.Params {
			params[i] = funcResolverParam{name: p.Name, t: componentValTypeResolver(p.Type)}
		}
		results := make([]funcResolverParam, len(t.Results))
		for i, r := range t.Results {
			results[i] = funcResolverParam{name: r.Name, t: componentValTypeResolver(r.Type)}
		}
		return funcResolver{params: params, results: results}
	case *wasmparser.ResourceType:
		rr := resourceResolver{}
		if t.Dtor.Valid {
			rr.hasDtor = true
			rr.dtorFuncIdx = t.Dtor.Value
		}
		return rr
	case *wasmparser.ComponentTypeDecl:
		return componentTypeResolver{}
	case *wasmparser.InstanceTypeDecl:
		return instanceTypeResolver{}
	}
	panic(fmt.Sprintf("wacogo: unhandled ComponentType %T", ct))
}
