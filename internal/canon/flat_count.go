package canon

// flatCountVisitor counts flat slots without emitting steps. Used for
// flat-vs-mem mode selection and variant joined-slot sizing.
type flatCountVisitor struct {
	count uint32
}

func (v *flatCountVisitor) add(n uint32) { v.count += n }

func (v *flatCountVisitor) VisitBool()               { v.add(1) }
func (v *flatCountVisitor) VisitU8()                 { v.add(1) }
func (v *flatCountVisitor) VisitU16()                { v.add(1) }
func (v *flatCountVisitor) VisitU32()                { v.add(1) }
func (v *flatCountVisitor) VisitU64()                { v.add(1) }
func (v *flatCountVisitor) VisitS8()                 { v.add(1) }
func (v *flatCountVisitor) VisitS16()                { v.add(1) }
func (v *flatCountVisitor) VisitS32()                { v.add(1) }
func (v *flatCountVisitor) VisitS64()                { v.add(1) }
func (v *flatCountVisitor) VisitF32()                { v.add(1) }
func (v *flatCountVisitor) VisitF64()                { v.add(1) }
func (v *flatCountVisitor) VisitChar()               { v.add(1) }
func (v *flatCountVisitor) VisitString()             { v.add(2) }
func (v *flatCountVisitor) VisitList(Type)           { v.add(2) }
func (v *flatCountVisitor) VisitFlags([]string)      { v.add(1) }
func (v *flatCountVisitor) VisitOwn(ResourceType)    { v.add(1) }
func (v *flatCountVisitor) VisitBorrow(ResourceType) { v.add(1) }
func (v *flatCountVisitor) VisitEnum(uint32)         { v.add(1) }
func (v *flatCountVisitor) VisitRecord(fields []RecordField) {
	for _, f := range fields {
		f.Type.Accept(v)
	}
}
func (v *flatCountVisitor) VisitTuple(types []Type) {
	for _, t := range types {
		t.Accept(v)
	}
}
func (v *flatCountVisitor) VisitVariant(cases []VariantCase) {
	var max uint32
	for _, c := range cases {
		if c.Payload == nil {
			continue
		}
		sub := &flatCountVisitor{}
		c.Payload.Accept(sub)
		if sub.count > max {
			max = sub.count
		}
	}
	v.add(1 + max) // disc + joined payload
}
func (v *flatCountVisitor) VisitOption(inner Type) {
	v.VisitVariant([]VariantCase{{Name: "none"}, {Name: "some", Payload: inner}})
}
func (v *flatCountVisitor) VisitResult(ok, err Type) {
	v.VisitVariant([]VariantCase{{Name: "ok", Payload: ok}, {Name: "err", Payload: err}})
}

// flatCountForType returns the flat-mode slot count of t. Used by
// variant flat-mode slot reservation (flatTransferVisitor.VisitVariant).
func flatCountForType(t Type) uint32 {
	fc := &flatCountVisitor{}
	t.Accept(fc)
	return fc.count
}

// FlatCount is the public counterpart of flatCountForType. Returns the
// canonical-ABI flat-mode slot count of t, counting each flat-register
// slot as 1. Used by external callers (e.g. the host package) to decide
// between flat-mode and memory-indirect canonical-ABI signatures.
func FlatCount(t Type) uint32 { return flatCountForType(t) }
