// Canonical-ABI metadata: byte size, alignment, flat slot count.
// Per the canonical ABI spec, primitives have natural sizes,
// strings and lists are (i32, i32), and tuples follow C-style
// layout with per-field alignment padding.
package witgen

import "fmt"

// Size returns the byte size in caller memory of a value of type t.
func Size(t Type) uint32 {
	switch v := t.(type) {
	case Prim:
		return primSize(v)
	case TypeString:
		return 8 // ptr + len, both i32
	case *TypeList:
		return 8 // ptr + len, both i32
	case *TypeTuple:
		var off uint32
		for _, f := range v.Fields {
			off = alignUp(off, Align(f))
			off += Size(f)
		}
		return alignUp(off, Align(t))
	case *TypeOption:
		a := Align(v.Elem)
		return alignUp(1, a) + Size(v.Elem)
	case *TypeResult:
		a := resultPayloadAlign(v)
		var maxArm uint32
		if s := resultArmSize(v.OK); s > maxArm {
			maxArm = s
		}
		if s := resultArmSize(v.Err); s > maxArm {
			maxArm = s
		}
		if maxArm == 0 {
			return 1 // disc only, when both arms void
		}
		return alignUp(1, a) + maxArm
	case *TypeEnum:
		if len(v.Cases) > 256 {
			panic(fmt.Sprintf("witgen: enum %q has %d cases; B-3 caps at 256", v.Name, len(v.Cases)))
		}
		return 1
	case *TypeFlags:
		if len(v.Cases) > 32 {
			panic(fmt.Sprintf("witgen: flags %q has %d cases; B-3 caps at 32", v.Name, len(v.Cases)))
		}
		return 4
	case *TypeRecord:
		var off uint32
		for _, f := range v.Fields {
			off = alignUp(off, Align(f.Type))
			off += Size(f.Type)
		}
		return alignUp(off, Align(t))
	case *TypeVariant:
		payloadAlign := variantPayloadAlign(v)
		var maxPayloadSize uint32
		for _, c := range v.Cases {
			if c.Payload != nil {
				if s := Size(c.Payload); s > maxPayloadSize {
					maxPayloadSize = s
				}
			}
		}
		if maxPayloadSize == 0 {
			return alignUp(1, Align(t)) // disc only, aligned to overall
		}
		return alignUp(alignUp(1, payloadAlign) + maxPayloadSize, Align(t))
	case *TypeResource, *TypeOwn, *TypeBorrow:
		return 4
	}
	panic("witgen: Size: unhandled type")
}

// Align returns the byte alignment requirement of a value of type t.
func Align(t Type) uint32 {
	switch v := t.(type) {
	case Prim:
		return primAlign(v)
	case TypeString:
		return 4
	case *TypeList:
		return 4
	case *TypeTuple:
		var a uint32 = 1
		for _, f := range v.Fields {
			if fa := Align(f); fa > a {
				a = fa
			}
		}
		return a
	case *TypeOption:
		return Align(v.Elem)
	case *TypeResult:
		return resultPayloadAlign(v)
	case *TypeEnum:
		return 1
	case *TypeFlags:
		return 4
	case *TypeRecord:
		var a uint32 = 1
		for _, f := range v.Fields {
			if fa := Align(f.Type); fa > a {
				a = fa
			}
		}
		return a
	case *TypeVariant:
		return variantPayloadAlign(v)
	case *TypeResource, *TypeOwn, *TypeBorrow:
		return 4
	}
	panic("witgen: Align: unhandled type")
}

// FlatSlots returns the number of canonical-ABI flat slots a value of
// type t occupies on the wasm stack (when in flat mode).
func FlatSlots(t Type) int {
	switch v := t.(type) {
	case Prim:
		return 1
	case TypeString:
		return 2
	case *TypeList:
		return 2
	case *TypeTuple:
		n := 0
		for _, f := range v.Fields {
			n += FlatSlots(f)
		}
		return n
	case *TypeOption:
		return 1 + FlatSlots(v.Elem)
	case *TypeResult:
		return 1 + resultMaxArmSlots(v)
	case *TypeEnum, *TypeFlags:
		return 1
	case *TypeRecord:
		n := 0
		for _, f := range v.Fields {
			n += FlatSlots(f.Type)
		}
		return n
	case *TypeVariant:
		var maxSlots int
		for _, c := range v.Cases {
			if c.Payload != nil {
				if s := FlatSlots(c.Payload); s > maxSlots {
					maxSlots = s
				}
			}
		}
		return 1 + maxSlots
	case *TypeResource, *TypeOwn, *TypeBorrow:
		return 1
	}
	panic("witgen: FlatSlots: unhandled type")
}

// FieldOffset returns the byte offset (within a tuple/record's memory
// layout) of the field at idx. Fields are packed with per-field
// alignment padding, C-style.
func FieldOffset(fields []Type, idx int) uint32 {
	var off uint32
	for i, f := range fields {
		off = alignUp(off, Align(f))
		if i == idx {
			return off
		}
		off += Size(f)
	}
	panic("witgen: FieldOffset: idx out of range")
}

func alignUp(off, a uint32) uint32 {
	return (off + a - 1) &^ (a - 1)
}

func primSize(p Prim) uint32 {
	switch p {
	case PrimBool, PrimU8, PrimS8:
		return 1
	case PrimU16, PrimS16:
		return 2
	case PrimU32, PrimS32, PrimF32, PrimChar:
		return 4
	case PrimU64, PrimS64, PrimF64:
		return 8
	}
	panic("witgen: primSize: unknown Prim")
}

func primAlign(p Prim) uint32 {
	return primSize(p) // Primitives align to their size in canon.
}

func resultPayloadAlign(r *TypeResult) uint32 {
	var a uint32 = 1
	if r.OK != nil {
		if aa := Align(r.OK); aa > a {
			a = aa
		}
	}
	if r.Err != nil {
		if aa := Align(r.Err); aa > a {
			a = aa
		}
	}
	return a
}

func resultArmSize(t Type) uint32 {
	if t == nil {
		return 0
	}
	return Size(t)
}

func resultMaxArmSlots(r *TypeResult) int {
	okN, errN := 0, 0
	if r.OK != nil {
		okN = FlatSlots(r.OK)
	}
	if r.Err != nil {
		errN = FlatSlots(r.Err)
	}
	if okN > errN {
		return okN
	}
	return errN
}

// variantPayloadAlign returns max(case payload aligns, 1).
func variantPayloadAlign(v *TypeVariant) uint32 {
	var a uint32 = 1
	for _, c := range v.Cases {
		if c.Payload != nil {
			if ca := Align(c.Payload); ca > a {
				a = ca
			}
		}
	}
	return a
}
