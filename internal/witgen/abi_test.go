package witgen

import "testing"

func TestSize(t *testing.T) {
	cases := []struct {
		t    Type
		want uint32
	}{
		{PrimBool, 1},
		{PrimU8, 1},
		{PrimU16, 2},
		{PrimU32, 4},
		{PrimU64, 8},
		{PrimS8, 1},
		{PrimS32, 4},
		{PrimS64, 8},
		{PrimF32, 4},
		{PrimF64, 8},
		{PrimChar, 4},
		{TypeString{}, 8},                                  // ptr + len
		{&TypeList{Elem: PrimU32}, 8},                       // ptr + len
		{&TypeTuple{Fields: []Type{PrimU32, PrimU32}}, 8},   // 4 + 4
		{&TypeTuple{Fields: []Type{PrimU8, PrimU32}}, 8},    // 1 padded to 4 + 4
		{&TypeTuple{Fields: []Type{PrimU32, PrimU64}}, 16},  // 4 padded to 8 + 8
		{&TypeTuple{Fields: []Type{TypeString{}, PrimU32}}, 12}, // 8 + 4
	}
	for _, c := range cases {
		if got := Size(c.t); got != c.want {
			t.Errorf("Size(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func TestAlign(t *testing.T) {
	cases := []struct {
		t    Type
		want uint32
	}{
		{PrimBool, 1},
		{PrimU16, 2},
		{PrimU32, 4},
		{PrimU64, 8},
		{PrimChar, 4},
		{TypeString{}, 4},
		{&TypeList{Elem: PrimU64}, 4},                       // ptr+len both u32
		{&TypeTuple{Fields: []Type{PrimU8, PrimU32}}, 4},
		{&TypeTuple{Fields: []Type{PrimU32, PrimU64}}, 8},
	}
	for _, c := range cases {
		if got := Align(c.t); got != c.want {
			t.Errorf("Align(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func TestFlatSlots(t *testing.T) {
	cases := []struct {
		t    Type
		want int
	}{
		{PrimU32, 1},
		{PrimU64, 1},
		{TypeString{}, 2},
		{&TypeList{Elem: PrimU32}, 2},
		{&TypeTuple{Fields: []Type{PrimU32, PrimU32}}, 2},
		{&TypeTuple{Fields: []Type{TypeString{}, PrimU32}}, 3},
	}
	for _, c := range cases {
		if got := FlatSlots(c.t); got != c.want {
			t.Errorf("FlatSlots(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func TestFieldOffset(t *testing.T) {
	fs := []Type{PrimU8, PrimU32, PrimU64}
	cases := []struct{ idx int; want uint32 }{
		{0, 0}, {1, 4}, {2, 8},
	}
	for _, c := range cases {
		if got := FieldOffset(fs, c.idx); got != c.want {
			t.Errorf("FieldOffset(%v, %d) = %d, want %d", fs, c.idx, got, c.want)
		}
	}
}

func TestSize_NewTypes(t *testing.T) {
	cases := []struct {
		t    Type
		want uint32
	}{
		{&TypeOption{Elem: PrimU32}, 8},                            // 1 disc + 3 pad + 4 = 8
		{&TypeOption{Elem: PrimU8}, 2},                              // 1 disc + 1 = 2
		{&TypeResult{OK: PrimU32, Err: PrimU64}, 16},                // 1 + 7 pad + 8
		{&TypeResult{}, 1},                                          // disc only
		{&TypeEnum{Name: "color", Cases: make([]EnumCase, 3)}, 1},   // u8
		{&TypeFlags{Name: "perms", Cases: make([]FlagCase, 3)}, 4},  // u32
	}
	for _, c := range cases {
		if got := Size(c.t); got != c.want {
			t.Errorf("Size(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func TestAlign_NewTypes(t *testing.T) {
	cases := []struct {
		t    Type
		want uint32
	}{
		{&TypeOption{Elem: PrimU64}, 8},
		{&TypeResult{OK: PrimU32, Err: TypeString{}}, 4},
		{&TypeEnum{}, 1},
		{&TypeFlags{}, 4},
	}
	for _, c := range cases {
		if got := Align(c.t); got != c.want {
			t.Errorf("Align(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func TestFlatSlots_NewTypes(t *testing.T) {
	cases := []struct {
		t    Type
		want int
	}{
		{&TypeOption{Elem: PrimU32}, 2},                              // 1 disc + 1
		{&TypeResult{OK: PrimU32, Err: TypeString{}}, 3},             // 1 disc + max(1, 2)
		{&TypeEnum{}, 1},
		{&TypeFlags{}, 1},
	}
	for _, c := range cases {
		if got := FlatSlots(c.t); got != c.want {
			t.Errorf("FlatSlots(%v) = %d, want %d", c.t, got, c.want)
		}
	}
}

func TestSize_Record(t *testing.T) {
	rec := &TypeRecord{Fields: []RecordField{
		{Type: PrimU8},
		{Type: PrimU32},
	}}
	if got := Size(rec); got != 8 {
		t.Errorf("Size(record{u8,u32}) = %d, want 8", got)
	}
}

func TestAlign_Record(t *testing.T) {
	rec := &TypeRecord{Fields: []RecordField{
		{Type: PrimU32},
		{Type: PrimU64},
	}}
	if got := Align(rec); got != 8 {
		t.Errorf("Align(record{u32,u64}) = %d, want 8", got)
	}
}

func TestFlatSlots_Record(t *testing.T) {
	rec := &TypeRecord{Fields: []RecordField{
		{Type: PrimU32},
		{Type: TypeString{}},
	}}
	if got := FlatSlots(rec); got != 3 {
		t.Errorf("FlatSlots(record{u32,string}) = %d, want 3", got)
	}
}

func TestSize_Variant(t *testing.T) {
	// shape with circle(u32), rect({u32,u32}), none
	v := &TypeVariant{Cases: []VariantCase{
		{Payload: PrimU32},                                   // 4 bytes
		{Payload: &TypeTuple{Fields: []Type{PrimU32, PrimU32}}}, // 8 bytes
		{},                                                    // 0 bytes (no payload)
	}}
	// 1 disc + 3 pad (align to 4) + 8 max payload = 12
	if got := Size(v); got != 12 {
		t.Errorf("Size(variant) = %d, want 12", got)
	}
}

func TestAlign_Variant(t *testing.T) {
	v := &TypeVariant{Cases: []VariantCase{
		{Payload: PrimU64},
		{Payload: PrimU32},
	}}
	if got := Align(v); got != 8 {
		t.Errorf("Align(variant) = %d, want 8", got)
	}
}

func TestFlatSlots_Variant(t *testing.T) {
	v := &TypeVariant{Cases: []VariantCase{
		{Payload: PrimU32},
		{Payload: TypeString{}}, // 2 slots
		{},
	}}
	// 1 disc + max(1, 2, 0) = 3
	if got := FlatSlots(v); got != 3 {
		t.Errorf("FlatSlots(variant) = %d, want 3", got)
	}
}

func TestSize_ResourceHandles(t *testing.T) {
	rt := &TypeResource{GoName: "Counter"}
	if Size(&TypeOwn{Resource: rt}) != 4 {
		t.Errorf("Size(own) != 4")
	}
	if Size(&TypeBorrow{Resource: rt}) != 4 {
		t.Errorf("Size(borrow) != 4")
	}
	if Align(&TypeOwn{Resource: rt}) != 4 {
		t.Errorf("Align(own) != 4")
	}
	if FlatSlots(&TypeOwn{Resource: rt}) != 1 {
		t.Errorf("FlatSlots(own) != 1")
	}
}
