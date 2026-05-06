package host

import "github.com/partite-ai/wacogo/internal/core"

// CallContext is the per-call value passed to every host Func. It
// exposes the caller memory and resource-table operations needed to
// read and write canonical-ABI arguments and results.
type CallContext = core.CallContext

// TypeExpr describes a component-model value type when declaring
// host-component signatures. Construct one by using the primitive
// variables (Bool, U32, String, ...), the compound types (Record,
// Variant, List, Tuple, Option, Result, Flags, Enum), or a *TypeRef
// handle returned by Builder.AddType or ResourceType.Own / Borrow.
//
// Nominal compounds (Record, Variant, Flags, Enum) are legal only as
// the top-level argument to Builder.AddType; using one inline inside
// a FuncType parameter or a compound is rejected at Build time.
// Structural compounds (List, Tuple, Option, Result) may be used
// inline or registered via AddType for sharing. Primitives and
// own/borrow handles are always used inline.
type TypeExpr interface {
	isTypeExpr()
}

// prim wraps a core primitive. Constructed only via the package-level
// primitive variables.
type prim struct {
	t core.Type
}

func (prim) isTypeExpr() {}

// Bool, U8 through U64, S8 through S64, F32, F64, Char, and String
// are the TypeExpr values for each component-model primitive type.
// Use them directly anywhere a TypeExpr is accepted. Primitives
// cannot be registered via Builder.AddType.
var (
	Bool   TypeExpr = prim{core.TypeBool{}}
	U8     TypeExpr = prim{core.TypeU8{}}
	U16    TypeExpr = prim{core.TypeU16{}}
	U32    TypeExpr = prim{core.TypeU32{}}
	U64    TypeExpr = prim{core.TypeU64{}}
	S8     TypeExpr = prim{core.TypeS8{}}
	S16    TypeExpr = prim{core.TypeS16{}}
	S32    TypeExpr = prim{core.TypeS32{}}
	S64    TypeExpr = prim{core.TypeS64{}}
	F32    TypeExpr = prim{core.TypeF32{}}
	F64    TypeExpr = prim{core.TypeF64{}}
	Char   TypeExpr = prim{core.TypeChar{}}
	String TypeExpr = prim{core.TypeString{}}
)

// TypeRef is an opaque handle to a previously-declared type. Use it
// anywhere a TypeExpr is accepted. Obtain one from Builder.AddType,
// ResourceType.Own / ResourceType.Borrow, or ResourceTypeRef.Own /
// ResourceTypeRef.Borrow. Identity is by pointer value: each call
// returns a distinct TypeRef.
type TypeRef struct {
	declExpr TypeExpr

	ownOfResource    *ResourceType
	borrowOfResource *ResourceType

	ownOfResourceRef    *ResourceTypeRef
	borrowOfResourceRef *ResourceTypeRef
}

func (*TypeRef) isTypeExpr() {}

// Record declares a record type. Record is nominal and must be
// registered with Builder.AddType; inline use is rejected at Build
// time.
type Record struct {
	Fields []Field
}

// Variant declares a variant (tagged union) type. A Case may leave
// Payload nil to declare a payloadless case. Variant is nominal and
// must be registered with Builder.AddType; inline use is rejected at
// Build time.
type Variant struct {
	Cases []Case
}

// List declares a homogeneous list of elements of type Elem. List
// is structural and may be used inline or registered with
// Builder.AddType.
type List struct {
	Elem TypeExpr
}

// Tuple declares a fixed-arity heterogeneous sequence. Tuple is
// structural and may be used inline or registered with
// Builder.AddType.
type Tuple struct {
	Types []TypeExpr
}

// Option declares an optional value (none | some(Inner)). Option is
// structural and may be used inline or registered with
// Builder.AddType.
type Option struct {
	Inner TypeExpr
}

// Result declares a result<Ok, Err> value. Either Ok or Err (or both)
// may be nil to declare a payloadless arm. Result is structural and
// may be used inline or registered with Builder.AddType.
type Result struct {
	Ok  TypeExpr
	Err TypeExpr
}

// Flags declares a set of named boolean flags. Flags is nominal and
// must be registered with Builder.AddType.
type Flags struct {
	Names []string
}

// Enum declares a fixed set of named cases with no payloads. Enum
// is nominal and must be registered with Builder.AddType.
type Enum struct {
	Cases []string
}

func (Record) isTypeExpr()  {}
func (Variant) isTypeExpr() {}
func (List) isTypeExpr()    {}
func (Tuple) isTypeExpr()   {}
func (Option) isTypeExpr()  {}
func (Result) isTypeExpr()  {}
func (Flags) isTypeExpr()   {}
func (Enum) isTypeExpr()    {}

// Field is one named field in a Record declaration. Name must be
// unique within the containing Record; Type may be any TypeExpr.
type Field struct {
	Name string
	Type TypeExpr
}

// Case is one named case in a Variant declaration. Name must be
// unique within the containing Variant. Payload is nil for
// payloadless cases.
type Case struct {
	Name    string
	Payload TypeExpr
}

// FuncType is a function signature used with Builder.AddFunction.
// Params and Results may each reference any TypeExpr subject to the
// same nominal/structural/inline rules as elsewhere.
type FuncType struct {
	Params  []Param
	Results []ResultDecl
}

// Param is one named function parameter.
type Param struct {
	Name string
	Type TypeExpr
}

// ResultDecl is one function result. Name is empty for unnamed results.
type ResultDecl struct {
	Name string
	Type TypeExpr
}
