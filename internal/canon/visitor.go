package canon

// Type is the canonical-ABI type interface consumed by visitors. Each parent-
// package Type implements Accept by dispatching to the right Visit* method.
type Type interface {
	Accept(v TypeVisitor)
}

// TypeVisitor is driven by Type.Accept. Each of the six concrete visitors
// (flatTransferVisitor, memTransferVisitor, valToFlatVisitor, valToMemVisitor,
// flatToValVisitor, memToValVisitor) implements this interface and emits
// closures into a per-visitor out slice.
type TypeVisitor interface {
	// Primitives
	VisitBool()
	VisitU8()
	VisitU16()
	VisitU32()
	VisitU64()
	VisitS8()
	VisitS16()
	VisitS32()
	VisitS64()
	VisitF32()
	VisitF64()
	VisitChar()
	VisitString()

	// Composites
	VisitList(elem Type)
	VisitRecord(fields []RecordField)
	VisitVariant(cases []VariantCase)
	VisitFlags(names []string)
	VisitOwn(rt ResourceType)
	VisitBorrow(rt ResourceType)

	// Sugar (layout-equivalent to composites but with distinct Val shapes)
	VisitOption(inner Type)
	VisitResult(ok, err Type) // either or both may be nil
	VisitTuple(types []Type)
	VisitEnum(numCases uint32)
}

// RecordField describes one record field the visitor will walk.
type RecordField struct {
	Name string
	Type Type
}

// VariantCase describes one variant case. Payload is nil for no-payload
// cases. Name is retained for diagnostics; visitors key on discriminant index.
type VariantCase struct {
	Name    string
	Payload Type
}
