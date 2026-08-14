package core

import "context"

// Type describes a component model value type.
type Type interface {
	typ()
}

// --- Primitive type descriptors ---

type TypeBool struct{}
type TypeS8 struct{}
type TypeU8 struct{}
type TypeS16 struct{}
type TypeU16 struct{}
type TypeS32 struct{}
type TypeU32 struct{}
type TypeS64 struct{}
type TypeU64 struct{}
type TypeF32 struct{}
type TypeF64 struct{}
type TypeChar struct{}
type TypeString struct{}

func (TypeBool) typ()   {}
func (TypeS8) typ()     {}
func (TypeU8) typ()     {}
func (TypeS16) typ()    {}
func (TypeU16) typ()    {}
func (TypeS32) typ()    {}
func (TypeU32) typ()    {}
func (TypeS64) typ()    {}
func (TypeU64) typ()    {}
func (TypeF32) typ()    {}
func (TypeF64) typ()    {}
func (TypeChar) typ()   {}
func (TypeString) typ() {}

// --- Compound type descriptors ---

// TypeList is a list of elements of type Elem.
type TypeList struct{ Elem Type }

func (TypeList) typ() {}

// TypeRecord is a record (struct) with named fields.
type TypeRecord struct{ Fields []FieldType }

func (TypeRecord) typ() {}

// TypeTuple is an anonymous tuple of types.
type TypeTuple struct{ Types []Type }

func (TypeTuple) typ() {}

// TypeVariant is a tagged union with named cases and optional payloads.
type TypeVariant struct{ Cases []CaseType }

func (TypeVariant) typ() {}

// TypeEnum is a discriminated union with no payloads (pure enum).
type TypeEnum struct{ Cases []string }

func (TypeEnum) typ() {}

// TypeOption is an optional type (none | some(Inner)).
type TypeOption struct{ Inner Type }

func (TypeOption) typ() {}

// TypeResult is a result type (ok(Ok) | err(Err)). Either Ok or Err may be nil.
type TypeResult struct {
	Ok  Type
	Err Type
}

func (TypeResult) typ() {}

// TypeFlags is a set of named boolean flags.
type TypeFlags struct{ Names []string }

func (TypeFlags) typ() {}

// TypeOwn is an owned resource handle. ResourceType identifies the nominal
// resource type of the referenced handle by pointer equality.
type TypeOwn struct{ ResourceType *TypeResource }

func (TypeOwn) typ() {}

// TypeBorrow is a borrowed resource handle. ResourceType identifies the
// nominal resource type of the referenced handle by pointer equality.
type TypeBorrow struct{ ResourceType *TypeResource }

func (TypeBorrow) typ() {}

// TypeResource is a nominal resource type. Identity is pointer equality on
// *TypeResource — two handles represent the same resource type iff they point
// to the same struct. The defining instance is carried so consumers can reach
// the origin Component via instance.component; dtor is resolved at resolver
// run time (unresolvable dtor is a load error, not deferred).
//
// dtor is the Go-callable destructor canon's ResourceTable.Drop invokes
// via ResourceType.Destructor when the last own handle for the resource
// type is dropped. Wasm-defined resources wrap their api.Function into
// this closure shape at construction time; host-impl resources supply
// the closure directly. nil means no destructor.
type TypeResource struct {
	instance *ComponentInstance
	dtor     func(ctx context.Context, rep uint32) error
}

func (*TypeResource) typ() {}

// NewTypeResource returns a *TypeResource defined by inst — the
// instance whose resource table holds its handles and whose reentrance
// gate wraps its destructor. Pass a nil dtor for resources with no
// destructor.
//
// inst is the instance that introduces the type, which is not
// necessarily the one that exports the name: an instance assembled
// purely from exports resolves names over types defined by its
// enclosing instance.
//
// dtor is the single Go-callable destructor canon's ResourceTable.Drop
// invokes via ResourceType.Destructor when the last own handle for
// the resource type is dropped. Wasm callers must wrap their
// api.Function into this closure shape at construction time; host-impl
// resources supply the closure directly.
func NewTypeResource(inst *ComponentInstance, dtor func(ctx context.Context, rep uint32) error) *TypeResource {
	return &TypeResource{instance: inst, dtor: dtor}
}

// Instance returns the *ComponentInstance that defines this resource
// type. Nil for TRs that have not yet been bound to an instance.
func (tr *TypeResource) Instance() *ComponentInstance { return tr.instance }

// InstanceType is a placeholder Type used to occupy TypeID slots allocated
// for spec-level instance type definitions. Empty for now; fields may be
// added later if the ABI grows a consumer.
type InstanceType struct{}

func (*InstanceType) typ() {}

// ComponentType is a placeholder Type for spec-level component type definitions.
// Empty for now.
type ComponentType struct{}

func (*ComponentType) typ() {}

// --- Supporting types ---

// FieldType is a named field in a record type.
type FieldType struct {
	Name string
	Type Type
}

// CaseType is a named case in a variant type. Payload may be nil.
type CaseType struct {
	Name    string
	Payload Type
}

// ParamType is a named parameter in a function type.
type ParamType struct {
	Name string
	Type Type
}

// ResultType is a (possibly unnamed) result in a function type. Name may be empty.
type ResultType struct {
	Name string
	Type Type
}

// FuncType is a component model function type.
type FuncType struct {
	Params  []ParamType
	Results []ResultType
}

func (*FuncType) typ() {}
