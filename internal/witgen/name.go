package witgen

import (
	"fmt"
	"strings"
)

// initialisms is the table of segments that should be UPPER-CASED in
// exported identifiers. Order doesn't matter; lookups are case-insensitive
// (against the lowercased segment).
var initialisms = map[string]string{
	"id":   "ID",
	"url":  "URL",
	"http": "HTTP",
	"json": "JSON",
	"uuid": "UUID",
}

// GoName converts a kebab-case WIT identifier into a Go identifier.
// If exported is true, the first segment is also uppercased; otherwise
// the first segment is lowercased (and never replaced by an initialism,
// since that would break exporting rules).
func GoName(s string, exported bool) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "-")
	var sb strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 && !exported {
			sb.WriteString(strings.ToLower(p))
			continue
		}
		if up, ok := initialisms[strings.ToLower(p)]; ok {
			sb.WriteString(up)
			continue
		}
		sb.WriteString(strings.ToUpper(p[:1]))
		sb.WriteString(p[1:])
	}
	return sb.String()
}

// goPackageNameRemap maps WIT interface names whose natural Go package
// identifier would shadow a Go builtin or keyword to a safe alternative.
// Add entries here as needed when new WASI interfaces trigger the collision.
var goPackageNameRemap = map[string]string{
	"error": "wioerror", // wasi:io/error → "error" shadows builtin
}

// GoPackageName produces a Go package identifier from a kebab-case
// WIT identifier by removing dashes and lowercasing. If the resulting
// identifier shadows a Go builtin or keyword, a safe remapped name is
// returned instead.
func GoPackageName(s string) string {
	raw := strings.ToLower(strings.ReplaceAll(s, "-", ""))
	if remap, ok := goPackageNameRemap[raw]; ok {
		return remap
	}
	return raw
}

// TypeName produces a PascalCase identifier suitable as a Go function
// suffix or a struct name for the given Type. Composite names are
// recursive concatenations.
func TypeName(t Type) string {
	switch v := t.(type) {
	case Prim:
		return primName(v)
	case TypeString:
		return "String"
	case *TypeList:
		return "List" + TypeName(v.Elem)
	case *TypeTuple:
		var sb strings.Builder
		if v.GoPackageQualifier != "" {
			sb.WriteString(GoName(v.GoPackageQualifier, true))
		}
		sb.WriteString("Tuple")
		for _, f := range v.Fields {
			sb.WriteString(TypeName(f))
		}
		return sb.String()
	case *TypeOption:
		if v.GoPackageQualifier != "" {
			return GoName(v.GoPackageQualifier, true) + "Option" + localTypeName(v.Elem)
		}
		return "Option" + TypeName(v.Elem)
	case *TypeResult:
		if v.GoPackageQualifier != "" {
			return GoName(v.GoPackageQualifier, true) + "Result" + resultArmName(v.OK) + resultArmName(v.Err)
		}
		return "Result" + resultArmName(v.OK) + resultArmName(v.Err)
	case *TypeEnum:
		if v.GoPackageQualifier != "" {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeFlags:
		if v.GoPackageQualifier != "" {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeRecord:
		if v.GoPackageQualifier != "" {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeVariant:
		if v.GoPackageQualifier != "" {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeResource:
		return v.GoName
	case *TypeOwn:
		return v.Resource.GoName
	case *TypeBorrow:
		return v.Resource.GoName
	}
	panic(fmt.Sprintf("witgen: TypeName: unhandled %T", t))
}

// localTypeName returns the unqualified name component for t, stripping
// the GoPackageQualifier prefix from nominal types. Equivalent to
// localTypeNameRel with the elem owner being the elem's own qualifier
// — i.e. as if the caller is the source package itself.
func localTypeName(t Type) string {
	return localTypeNameRel(t, qualifierOf(t))
}

// localTypeNameRel returns the name component for t as the OWNING package
// of an outer compound (qualifier ownerPkg) would name it. If t's own
// qualifier matches ownerPkg, the prefix is stripped (the type is local
// to ownerPkg from owner's view); otherwise the qualifier is kept.
//
// Used when constructing the Go reference name of a cross-package compound
// (option, tuple, result) so the synthesized name matches what the
// owner package actually emitted.
func localTypeNameRel(t Type, ownerPkg string) string {
	switch v := t.(type) {
	case *TypeRecord:
		if v.GoPackageQualifier != "" && v.GoPackageQualifier != ownerPkg {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeVariant:
		if v.GoPackageQualifier != "" && v.GoPackageQualifier != ownerPkg {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeEnum:
		if v.GoPackageQualifier != "" && v.GoPackageQualifier != ownerPkg {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeFlags:
		if v.GoPackageQualifier != "" && v.GoPackageQualifier != ownerPkg {
			return GoName(v.GoPackageQualifier, true) + v.GoName
		}
		return v.GoName
	case *TypeList:
		return "List" + localTypeNameRel(v.Elem, ownerPkg)
	case *TypeOption:
		return "Option" + localTypeNameRel(v.Elem, ownerPkg)
	case *TypeTuple:
		var sb strings.Builder
		sb.WriteString("Tuple")
		for _, f := range v.Fields {
			sb.WriteString(localTypeNameRel(f, ownerPkg))
		}
		return sb.String()
	case *TypeResult:
		return "Result" +
			localResultArmNameRel(v.OK, ownerPkg) +
			localResultArmNameRel(v.Err, ownerPkg)
	}
	// Primitives, TypeString, TypeResource, TypeOwn, TypeBorrow:
	// no qualifier-stripping needed.
	return TypeName(t)
}

// localResultArmNameRel returns the result-arm name component for t as
// the owning package (ownerPkg) would name it; "_" for nil.
func localResultArmNameRel(t Type, ownerPkg string) string {
	if t == nil {
		return "_"
	}
	return localTypeNameRel(t, ownerPkg)
}

// qualifierOf returns the GoPackageQualifier of t, or "" if t has none.
// Compounds without a qualifier field return "".
func qualifierOf(t Type) string {
	switch v := t.(type) {
	case *TypeRecord:
		return v.GoPackageQualifier
	case *TypeVariant:
		return v.GoPackageQualifier
	case *TypeEnum:
		return v.GoPackageQualifier
	case *TypeFlags:
		return v.GoPackageQualifier
	case *TypeOption:
		return v.GoPackageQualifier
	case *TypeTuple:
		return v.GoPackageQualifier
	case *TypeResult:
		return v.GoPackageQualifier
	}
	return ""
}

// resultArmName returns the name component for one result arm; "_" for nil.
func resultArmName(t Type) string {
	if t == nil {
		return "_"
	}
	return TypeName(t)
}

// ResultCaseGoName returns the Go expression for one case-struct of a
// Result type — e.g. "ResultFooOk" for a local Result, or
// "streams.ResultListU8StreamErrorErr" for a Result owned by the
// imported streams package. suffix is "Ok" or "Err".
func ResultCaseGoName(v *TypeResult, suffix string) string {
	if v.GoPackageQualifier != "" {
		localName := "Result" +
			localResultArmNameRel(v.OK, v.GoPackageQualifier) +
			localResultArmNameRel(v.Err, v.GoPackageQualifier)
		return qualifyType(v.GoPackageQualifier, localName) + suffix
	}
	return TypeName(v) + suffix
}

func primName(p Prim) string {
	switch p {
	case PrimBool:
		return "Bool"
	case PrimU8:
		return "U8"
	case PrimU16:
		return "U16"
	case PrimU32:
		return "U32"
	case PrimU64:
		return "U64"
	case PrimS8:
		return "S8"
	case PrimS16:
		return "S16"
	case PrimS32:
		return "S32"
	case PrimS64:
		return "S64"
	case PrimF32:
		return "F32"
	case PrimF64:
		return "F64"
	case PrimChar:
		return "Char"
	}
	panic("witgen: primName: unknown Prim")
}

// GoTypeOf returns the Go type expression for t, used in interface
// signatures and field types.
//
// Primitives map directly to Go scalar types (uint32, string, etc.).
// Lists are []ElemType. Tuples become a generated struct named per
// TypeName(t) — the struct declaration itself lives in iface.go,
// emitted by the type-collection pass. Resource types (own<R>,
// borrow<R>, bare <R>) render as *<R>Handle so that compound types
// embedding resources (e.g. list<own<R>>, record { c: own<R> }) carry
// the cross-boundary handle struct in their Go shape.
func GoTypeOf(t Type) string {
	switch v := t.(type) {
	case Prim:
		return v.GoType()
	case TypeString:
		return "string"
	case *TypeList:
		return "[]" + GoTypeOf(v.Elem)
	case *TypeTuple:
		if v.GoPackageQualifier != "" {
			// The tuple type lives in the imported package; reference it directly.
			return qualifyType(v.GoPackageQualifier, "Tuple"+func() string {
				var sb strings.Builder
				for _, f := range v.Fields {
					sb.WriteString(TypeName(f))
				}
				return sb.String()
			}())
		}
		return TypeName(t)
	case *TypeOption:
		if v.GoPackageQualifier != "" {
			// Qualified option: the struct lives in another package.
			// Use localTypeNameRel so foreign-to-owner elem qualifiers
			// remain in the synthesized name (matching what the owner
			// package emitted), while elements local to the owner have
			// their qualifier stripped.
			localName := "Option" + localTypeNameRel(v.Elem, v.GoPackageQualifier)
			return qualifyType(v.GoPackageQualifier, localName)
		}
		return TypeName(t)
	case *TypeResult:
		if v.GoPackageQualifier != "" {
			localName := "Result" +
				localResultArmNameRel(v.OK, v.GoPackageQualifier) +
				localResultArmNameRel(v.Err, v.GoPackageQualifier)
			return qualifyType(v.GoPackageQualifier, localName)
		}
		return TypeName(t)
	case *TypeEnum:
		return qualifyType(v.GoPackageQualifier, v.GoName)
	case *TypeFlags:
		return qualifyType(v.GoPackageQualifier, v.GoName)
	case *TypeRecord:
		return qualifyType(v.GoPackageQualifier, v.GoName)
	case *TypeVariant:
		return qualifyType(v.GoPackageQualifier, v.GoName)
	case *TypeResource:
		return "*" + qualifyType(v.GoPackageQualifier, v.WrapName+"Handle")
	case *TypeOwn:
		return "*" + qualifyType(v.Resource.GoPackageQualifier, v.Resource.WrapName+"Handle")
	case *TypeBorrow:
		return "*" + qualifyType(v.Resource.GoPackageQualifier, v.Resource.WrapName+"Handle")
	}
	panic(fmt.Sprintf("witgen: GoTypeOf: unhandled %T", t))
}

// goTypeForSide renders the Go type expression for t. Any reference
// to a resource — own, borrow, or bare *TypeResource — renders as
// *<Resource>Handle (the cross-boundary handle struct). For imported
// resources (those with GoPackageQualifier set), the type is qualified
// with the package name (e.g., "*streams.HandleHandle").
//
// The useImpl parameter is retained for callsite compatibility but
// has no effect on resource rendering — every resource renders as a
// single *<R>Handle type.
func goTypeForSide(t Type, useImpl bool) string {
	switch v := t.(type) {
	case *TypeResource:
		return "*" + qualifyType(v.GoPackageQualifier, v.WrapName+"Handle")
	case *TypeOwn:
		return "*" + qualifyType(v.Resource.GoPackageQualifier, v.Resource.WrapName+"Handle")
	case *TypeBorrow:
		return "*" + qualifyType(v.Resource.GoPackageQualifier, v.Resource.WrapName+"Handle")
	default:
		return GoTypeOf(t)
	}
}

// qualifyType prepends pkg + "." to name if pkg is non-empty.
func qualifyType(pkg, name string) string {
	if pkg == "" {
		return name
	}
	return pkg + "." + name
}

// variantCaseName returns the package-qualified Go case-struct name for a
// variant case, using the variant's GoPackageQualifier when set. Used when
// emitting helper functions in an importing package to avoid bare references.
func variantCaseName(v *TypeVariant, c VariantCase) string {
	return qualifyType(v.GoPackageQualifier, c.GoName)
}

// ZeroValueExpr returns a Go expression producing the zero value of t.
// Used by emit-time errSinkLift and the wrap-method early-return path
// to produce a syntactically-valid zero value on failure.
func ZeroValueExpr(t Type) string {
	switch v := t.(type) {
	case Prim:
		switch v {
		case PrimBool:
			return "false"
		case PrimChar:
			return "0"
		}
		return "0" // numeric primitives
	case TypeString:
		return `""`
	case *TypeList:
		return "nil"
	case *TypeOption, *TypeRecord, *TypeTuple:
		return GoTypeOf(t) + "{}"
	case *TypeEnum:
		// Enum is a uint8 alias — zero value is the integer 0.
		return GoTypeOf(v) + "(0)"
	case *TypeFlags:
		// Flags is a uint32 alias — zero value is 0.
		return GoTypeOf(v) + "(0)"
	case *TypeResult:
		// Result is now a sealed interface; its zero value is nil.
		return "nil"
	case *TypeVariant:
		return "nil"
	case *TypeOwn:
		return "nil"
	case *TypeBorrow:
		return "nil"
	case *TypeResource:
		return "nil"
	}
	panic(fmt.Sprintf("witgen: ZeroValueExpr: unhandled %T", t))
}

// resourceFieldName returns the Factory field name for a resource type's
// *host.ResourceType, e.g., "counterRT" for resource with GoName "Counter".
func resourceFieldName(rt *TypeResource) string {
	if rt.GoName == "" {
		return "RT"
	}
	return strings.ToLower(rt.GoName[:1]) + rt.GoName[1:] + "RT"
}

