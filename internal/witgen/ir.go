package witgen

import "strings"

// Type is the interface satisfied by every IR type. Sealed via the
// unexported isType marker; new implementations live in this file.
type Type interface {
	isType()
}

func (Prim) isType()         {}
func (TypeString) isType()   {}
func (*TypeList) isType()    {}
func (*TypeTuple) isType()   {}
func (*TypeOption) isType()  {}
func (*TypeResult) isType()  {}
func (*TypeEnum) isType()    {}
func (*TypeFlags) isType()   {}
func (*TypeRecord) isType()  {}
func (*TypeVariant) isType() {}
func (*TypeResource) isType() {}
func (*TypeOwn) isType()      {}
func (*TypeBorrow) isType()   {}

// Prim enumerates the WIT primitive types.
type Prim int

const (
	PrimBool Prim = iota
	PrimU8
	PrimU16
	PrimU32
	PrimU64
	PrimS8
	PrimS16
	PrimS32
	PrimS64
	PrimF32
	PrimF64
	PrimChar
)

// GoType returns the Go type name for this primitive.
func (p Prim) GoType() string {
	switch p {
	case PrimBool:
		return "bool"
	case PrimU8:
		return "uint8"
	case PrimU16:
		return "uint16"
	case PrimU32:
		return "uint32"
	case PrimU64:
		return "uint64"
	case PrimS8:
		return "int8"
	case PrimS16:
		return "int16"
	case PrimS32:
		return "int32"
	case PrimS64:
		return "int64"
	case PrimF32:
		return "float32"
	case PrimF64:
		return "float64"
	case PrimChar:
		return "rune"
	}
	panic("witgen: unknown Prim")
}

// HostTypeExpr returns the host package's TypeExpr variable name for
// this primitive (e.g., "host.U32"). Used to materialize FuncType
// declarations in generated code.
func (p Prim) HostTypeExpr() string {
	switch p {
	case PrimBool:
		return "host.Bool"
	case PrimU8:
		return "host.U8"
	case PrimU16:
		return "host.U16"
	case PrimU32:
		return "host.U32"
	case PrimU64:
		return "host.U64"
	case PrimS8:
		return "host.S8"
	case PrimS16:
		return "host.S16"
	case PrimS32:
		return "host.S32"
	case PrimS64:
		return "host.S64"
	case PrimF32:
		return "host.F32"
	case PrimF64:
		return "host.F64"
	case PrimChar:
		return "host.Char"
	}
	panic("witgen: unknown Prim")
}

// TypeString is the WIT string primitive.
type TypeString struct{}

// GoType returns the Go type name for a string.
func (TypeString) GoType() string { return "string" }

// HostTypeExpr returns the host package's TypeExpr for a string.
func (TypeString) HostTypeExpr() string { return "host.String" }

// TypeList is a homogeneous list of Elem values.
type TypeList struct {
	Elem Type
}

// GoType returns the Go type name for a list.
func (l *TypeList) GoType() string { return GoTypeOf(l) }

// HostTypeExpr returns the host package's TypeExpr for a list.
func (l *TypeList) HostTypeExpr() string {
	return "host.List{Elem: " + hostTypeExpr(l.Elem) + "}"
}

// TypeTuple is a fixed-arity heterogeneous sequence of fields.
type TypeTuple struct {
	Fields             []Type
	GoPackageQualifier string // non-empty when the tuple is from an imported package
}

// GoType returns the Go type name for a tuple (the generated struct name).
func (t *TypeTuple) GoType() string { return GoTypeOf(t) }

// HostTypeExpr returns the host package's TypeExpr for a tuple.
func (t *TypeTuple) HostTypeExpr() string {
	var sb strings.Builder
	sb.WriteString("host.Tuple{Types: []host.TypeExpr{")
	for i, f := range t.Fields {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(hostTypeExpr(f))
	}
	sb.WriteString("}}")
	return sb.String()
}

// hostTypeExpr returns the host TypeExpr Go expression for any Type.
func hostTypeExpr(t Type) string {
	switch v := t.(type) {
	case Prim:
		return v.HostTypeExpr()
	case TypeString:
		return v.HostTypeExpr()
	case *TypeList:
		return v.HostTypeExpr()
	case *TypeTuple:
		return v.HostTypeExpr()
	case *TypeOption:
		return v.HostTypeExpr()
	case *TypeResult:
		return v.HostTypeExpr()
	case *TypeEnum:
		return v.HostTypeExpr()
	case *TypeFlags:
		return v.HostTypeExpr()
	case *TypeRecord:
		return v.HostTypeExpr()
	case *TypeVariant:
		return v.HostTypeExpr()
	case *TypeResource:
		return v.HostTypeExpr()
	case *TypeOwn:
		return v.HostTypeExpr()
	case *TypeBorrow:
		return v.HostTypeExpr()
	}
	panic("witgen: hostTypeExpr: unhandled type")
}

// Package is the top-level IR for one generation invocation: the chosen
// world plus the interfaces it imports.
type Package struct {
	Interfaces []*Interface
}

// ImportRef captures a cross-interface import — one source interface that
// the current interface references in any of its signatures. One ref per
// distinct source interface, regardless of how many resource types are
// pulled in.
type ImportRef struct {
	// Namespace, Package, Name identify the WIT interface being imported
	// (e.g. "example", "demo", "streams" for example:demo/streams).
	Namespace string
	Package   string
	Name      string

	// GoPackagePath is the fully qualified Go package path for the
	// dependency, used as the import path in generated code.
	// Example: "github.com/x/y/bindings/example/demo/streams".
	GoPackagePath string

	// GoPackage is the package name (used to qualify references like
	// streams.Stream).
	GoPackage string

	// GoFieldName is the exported PascalCase field name on the importer's
	// generated `Deps` struct that holds the dependency's
	// *wacogo.ComponentInstance (e.g., "Streams" for example:demo/streams).
	GoFieldName string

	// StateField is the unexported field name on the importer's generated
	// instanceState struct that holds the dependency's
	// *wacogo.ComponentInstance (e.g., "streams").
	StateField string

	// Resources lists the resources from this source interface that
	// the current interface references. Used to emit AddResourceRef
	// declarations and per-resource binding calls.
	Resources []ImportedResource

	// Records, Variants, Enums, Flags list nominals from this source
	// interface that the current interface references. Each appears at
	// most once per kind. Used to emit re-registration `b.AddType`
	// blocks in the importer's NewFactory.
	Records  []ImportedNominal
	Variants []ImportedNominal
	Enums    []ImportedNominal
	Flags    []ImportedNominal
}

// ImportedResource is one resource type pulled in via ImportRef.
type ImportedResource struct {
	Name     string // WIT name, e.g. "handle"
	WrapName string // Consumer-side Go type name (e.g. "Handle")
	RefField string // Field on Factory holding the *host.ResourceTypeRef (e.g. "HandleRef")

	// ForwarderType is the local Go type name for the importer-emitted
	// forwarder that satisfies the foreign <R> interface — e.g.
	// "counterhostCounterForwarder" when the importer pulls in
	// counterhost.Counter.
	ForwarderType string

	// FwdFnsType is the local Go type name for the precomputed per-method
	// *host.ExportedFunc table for the foreign resource — e.g.
	// "counterhostCounterFwdFns".
	FwdFnsType string

	// FwdFnsField is the unexported field name on the importer's
	// instanceState that holds *FwdFnsType, populated at NewInstance time.
	FwdFnsField string

	// Source is the original *TypeResource (in the owner interface)
	// this entry refers to, used by emitters to read the resource's
	// method list without re-walking the importing interface's
	// signatures.
	Source *TypeResource

	// QualifiedMethods is a per-importer copy of Source.Methods with
	// param and result types rewritten through the importer's qualifier
	// rewriter so emitted forwarder method signatures reference foreign
	// nominal types via their owning package qualifier.
	QualifiedMethods []ResourceMethod
}

// ImportedNominal is one nominal type (record, variant, enum, flags)
// pulled in via ImportRef. Source is a back-pointer to the original IR
// node in the owner interface; readers needing structural detail
// (record fields, variant cases, enum cases, flag bits) read through it.
//
// When Transitive is true, the nominal was pulled in only to support
// qualifier rewriting for an imported resource's method types — it is
// NOT directly referenced in the importing iface's own signatures.
// Factory emission skips AddType registration for transitive nominals;
// they appear only in resCopies/recCopies/etc. for type qualification.
type ImportedNominal struct {
	Name       string // WIT name, e.g. "point"
	GoName     string // Owner-package Go type name, e.g. "Point"
	Source     Type   // *TypeRecord | *TypeVariant | *TypeEnum | *TypeFlags
	Transitive bool
}

// Interface is one WIT interface that becomes one Go package.
type Interface struct {
	// Namespace, Package, Name are the WIT identity: foo:bar/baz =>
	// Namespace="foo", Package="bar", Name="baz".
	Namespace string
	Package   string
	Name      string

	// InterfaceName is the canonical WIT interface id, including any
	// version (e.g. "wasi:sockets/network@0.2.8" or
	// "example:demo/arith"). Emitted as a package-level constant in the
	// generated bindings.
	InterfaceName string

	// GoName holds the same value as WrapName. It exists so that template
	// references using GoName continue to compile; prefer WrapName in new code.
	GoName string

	// ImplName is the user-implementer interface's Go type name with the
	// "Impl" suffix (e.g., "StreamsImpl"). Users implement this when
	// constructing a host component that exports this interface.
	ImplName string

	// WrapName is the consumer-side Go type name (e.g., "Streams") returned
	// by WrapInstance. Code that holds a WrapName-typed value cannot
	// distinguish locally-implemented from remotely-wrapped instances.
	WrapName string

	// GoPackage is the Go package directory name (lowercase, no dashes).
	GoPackage string

	Funcs     []*Func
	Enums     []*TypeEnum
	Flags     []*TypeFlags
	Records   []*TypeRecord
	Variants  []*TypeVariant
	Resources []*TypeResource

	// Imports lists cross-interface imports referenced by this interface's
	// signatures. Empty for self-contained interfaces. Populated after
	// all interfaces in the package have been lowered.
	Imports []ImportRef

	// Docs is the WIT doc-comment text on this interface, with WIT '///'
	// prefixes stripped. Empty when the interface has no doc.
	Docs string
}

// Func is one WIT function on an interface.
type Func struct {
	WitName string
	GoName  string
	Params  []*Param
	Result  Type // nil for no result
	Docs    string
}

// Param is one named function parameter.
type Param struct {
	GoName string
	Type   Type
}

// TypeOption represents option<T>.
type TypeOption struct {
	Elem Type
	// GoPackageQualifier, when non-empty, indicates this option type is
	// imported from another package (e.g., "types" for types.OptionString).
	// The struct declaration lives in that package; the local package only
	// emits helper functions whose signatures and bodies use the qualified type.
	GoPackageQualifier string
}

// GoType returns the Go type expression for an option (the generated struct name).
func (o *TypeOption) GoType() string { return GoTypeOf(o) }

// HostTypeExpr returns the host package's TypeExpr for an option.
func (o *TypeOption) HostTypeExpr() string {
	return "host.Option{Inner: " + hostTypeExpr(o.Elem) + "}"
}

// TypeResult represents result<T, E>. Either arm may be nil for
// result<T> (no err payload), result<_, E> (no ok payload), or
// result (both void).
type TypeResult struct {
	OK  Type
	Err Type
	// GoPackageQualifier, when non-empty, indicates this result type is
	// owned by an imported package; signature renderers prefix the
	// generated struct/interface name with this package qualifier
	// (e.g. "streams.ResultListU8StreamError").
	GoPackageQualifier string
}

// GoType returns the Go type expression for a result (the generated struct name).
func (r *TypeResult) GoType() string { return GoTypeOf(r) }

// HostTypeExpr returns the host package's TypeExpr for a result.
func (r *TypeResult) HostTypeExpr() string {
	var sb strings.Builder
	sb.WriteString("host.Result{")
	wrote := false
	if r.OK != nil {
		sb.WriteString("Ok: ")
		sb.WriteString(hostTypeExpr(r.OK))
		wrote = true
	}
	if r.Err != nil {
		if wrote {
			sb.WriteString(", ")
		}
		sb.WriteString("Err: ")
		sb.WriteString(hostTypeExpr(r.Err))
	}
	sb.WriteString("}")
	return sb.String()
}

// EnumCase is one case of a WIT enum.
type EnumCase struct {
	Name   string // WIT case name, e.g. "red"
	GoName string // Go const name, e.g. "ColorRed"
	Docs   string
}

// TypeEnum is a nominal enum, declared once per interface.
// Referenced by any Func signature via the same *TypeEnum pointer
// (structural identity via pointer equality).
type TypeEnum struct {
	Name   string // WIT name, e.g. "color"
	GoName string // Go type name, e.g. "Color"
	Cases  []EnumCase

	// GoPackageQualifier, when non-empty, indicates this enum is imported
	// into the renderer's package; signature renderers prefix the type name
	// with this package qualifier (e.g. "streams.Color").
	GoPackageQualifier string

	// OwnerInterface points back at the interface that declares this nominal.
	// Always non-nil for nominals returned by LowerWorld.
	OwnerInterface *Interface

	Docs string
}

// HostTypeExpr returns the host package's TypeExpr for an enum.
// Enums are nominal and must be registered via Builder.AddType.
// The enum reference name is generated as "typ<EnumName>".
func (e *TypeEnum) HostTypeExpr() string {
	return "typ" + e.GoName + "_"
}

// FlagCase is one case of a WIT flags declaration.
type FlagCase struct {
	Name   string
	GoName string
	Docs   string
}

// TypeFlags is a nominal flags set, up to 32 cases in B-3.
type TypeFlags struct {
	Name   string
	GoName string
	Cases  []FlagCase

	// GoPackageQualifier, when non-empty, indicates this flags type is imported
	// into the renderer's package; signature renderers prefix the type name
	// with this package qualifier (e.g. "streams.Flags").
	GoPackageQualifier string

	// OwnerInterface points back at the interface that declares this nominal.
	// Always non-nil for nominals returned by LowerWorld.
	OwnerInterface *Interface

	Docs string
}

// HostTypeExpr returns the host package's TypeExpr for a flags set.
// Flags are nominal and must be registered via Builder.AddType.
// The flags reference name is generated as "typ<FlagsName>".
func (f *TypeFlags) HostTypeExpr() string {
	return "typ" + f.GoName + "_"
}

// RecordField is one named field in a record declaration.
type RecordField struct {
	Name   string
	GoName string
	Type   Type
	Docs   string
}

// TypeRecord is a nominal record, declared once per interface.
type TypeRecord struct {
	Name   string
	GoName string
	Fields []RecordField

	// GoPackageQualifier, when non-empty, indicates this record is imported
	// into the renderer's package; signature renderers prefix the type name
	// with this package qualifier (e.g. "streams.Foo").
	GoPackageQualifier string

	// OwnerInterface points back at the interface that declares this nominal.
	// Always non-nil for nominals returned by LowerWorld.
	OwnerInterface *Interface

	Docs string
}

// GoType returns the Go type name for a record (the generated struct name).
func (r *TypeRecord) GoType() string { return r.GoName }

// HostTypeExpr returns the host package's TypeExpr for a record.
// Records are nominal and must be registered via Builder.AddType.
// The record reference name is generated as "typ<RecordName>".
func (r *TypeRecord) HostTypeExpr() string {
	return "typ" + r.GoName + "_"
}

// VariantCase is one case in a variant declaration.
type VariantCase struct {
	Name    string // WIT name, e.g. "circle"
	GoName  string // Go case-struct name, e.g. "ShapeCircle"
	Payload Type   // nil for payloadless cases
	Docs    string
}

// TypeVariant is a nominal sum type, declared once per interface.
type TypeVariant struct {
	Name   string
	GoName string // Go marker interface name, e.g. "Shape"
	Cases  []VariantCase

	// GoPackageQualifier, when non-empty, indicates this variant is imported
	// into the renderer's package; signature renderers prefix the type name
	// with this package qualifier (e.g. "streams.Shape").
	GoPackageQualifier string

	// OwnerInterface points back at the interface that declares this nominal.
	// Always non-nil for nominals returned by LowerWorld.
	OwnerInterface *Interface

	Docs string
}

// GoType returns the Go type name for a variant (the generated interface name).
func (v *TypeVariant) GoType() string { return v.GoName }

// HostTypeExpr returns the host package's TypeExpr for a variant.
// Variants are nominal and must be registered via Builder.AddType.
// The variant reference name is generated as "typ<VariantName>".
func (v *TypeVariant) HostTypeExpr() string {
	return "typ" + v.GoName + "_"
}

// ResourceMethod is a per-instance method on a resource.
type ResourceMethod struct {
	Name   string // WIT name, e.g. "increment"
	GoName string // Go method name, e.g. "Increment"
	Params []*Param
	Result Type // nil for no result
	Docs   string
}

// ResourceCtor is a resource's constructor (zero-or-one per resource).
// The implicit result is own<R> pointing at the parent *TypeResource.
type ResourceCtor struct {
	Params []*Param
	Docs   string
}

// ResourceStatic is a static (no-receiver) function on a resource.
type ResourceStatic struct {
	Name   string
	GoName string // Go name on parent interface, e.g. "CounterOpen"
	Params []*Param
	Result Type
	Docs   string
}

// TypeResource is a nominal resource type.
type TypeResource struct {
	Name string

	// GoName holds the same value as WrapName. It exists so that template
	// references using GoName continue to compile; prefer WrapName in new code.
	GoName string

	// ImplName is the user-implementer Go interface name with the "Impl"
	// suffix (e.g., "CounterImpl"). Users implement this when their host
	// component owns this resource.
	ImplName string

	// WrapName is the consumer-side sealed Go interface name (e.g.,
	// "Counter"). Produced by WrapInstance and by the package's internal
	// newx<Name>(impl) helper.
	WrapName string

	// ImportRefField, when non-empty, indicates this resource is imported
	// from another interface. Its value is the Factory field name holding
	// the *host.ResourceTypeRef (e.g., "handleRef"). When empty the
	// resource is locally owned and uses resourceFieldName() to derive
	// the *host.ResourceType Factory field name.
	ImportRefField string

	// GoPackageQualifier, when non-empty, is the Go package qualifier to
	// prepend when referencing this resource's Go types in the importing
	// package (e.g., "streams" for "streams.Handle"). Set alongside
	// ImportRefField for imported resources.
	GoPackageQualifier string

	// LenderInstField, when non-empty, is the field name on the importing
	// package's generated instanceState struct that holds the bound lender
	// *wacogo.ComponentInstance (e.g., "streams"). Set alongside
	// ImportRefField for imported resources, derived from the ImportRef's
	// StateField.
	LenderInstField string

	Ctor    *ResourceCtor
	Methods []ResourceMethod
	Statics []ResourceStatic

	Docs string
}

// IsImported reports whether this resource type is imported from another
// interface (as opposed to locally owned by the current interface).
func (r *TypeResource) IsImported() bool { return r.ImportRefField != "" }

// HostTypeExpr returns the host package's TypeExpr for a resource type.
// Resources are nominal and registered via Builder.AddResource; the
// reference name is generated as "rt_<ResourceName>".
func (r *TypeResource) HostTypeExpr() string { return "rt_" + r.GoName }

// GoType returns the Go interface name for a resource.
func (r *TypeResource) GoType() string { return r.GoName }

// TypeOwn wraps a resource for own<R> handle semantics.
type TypeOwn struct{ Resource *TypeResource }

// HostTypeExpr returns the host package's TypeExpr for an own handle.
// For imported resources, uses the ResourceTypeRef field; for locally
// owned resources, uses the ResourceType field.
func (o *TypeOwn) HostTypeExpr() string {
	if o.Resource.ImportRefField != "" {
		return "f_." + o.Resource.ImportRefField + ".Own()"
	}
	return "f_." + resourceFieldName(o.Resource) + ".Own()"
}

// GoType returns the Go type name (the resource's interface name).
func (o *TypeOwn) GoType() string { return o.Resource.GoType() }

// TypeBorrow wraps a resource for borrow<R> handle semantics.
type TypeBorrow struct{ Resource *TypeResource }

// HostTypeExpr returns the host package's TypeExpr for a borrow handle.
// For imported resources, uses the ResourceTypeRef field; for locally
// owned resources, uses the ResourceType field.
func (b *TypeBorrow) HostTypeExpr() string {
	if b.Resource.ImportRefField != "" {
		return "f_." + b.Resource.ImportRefField + ".Borrow()"
	}
	return "f_." + resourceFieldName(b.Resource) + ".Borrow()"
}

// GoType returns the Go type name (the resource's interface name).
func (b *TypeBorrow) GoType() string { return b.Resource.GoType() }
