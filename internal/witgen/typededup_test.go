package witgen

import "testing"

func TestCollectTypes_DedupesAndOrders(t *testing.T) {
	iface := &Interface{
		Funcs: []*Func{
			{
				Params: []*Param{{Type: PrimU32}, {Type: TypeString{}}},
				Result: &TypeList{Elem: PrimU32},
			},
			{
				// Reuses TypeString and tuple<u32, string>.
				Params: []*Param{{Type: TypeString{}}},
				Result: &TypeTuple{Fields: []Type{PrimU32, TypeString{}}},
			},
			{
				// Same tuple shape — should dedup.
				Result: &TypeTuple{Fields: []Type{PrimU32, TypeString{}}},
			},
		},
	}
	types := CollectTypes(iface)

	// Should include: PrimU32, TypeString, ListU32, TupleU32String.
	// Order: by TypeName (deterministic).
	wantNames := []string{"ListU32", "String", "TupleU32String", "U32"}
	if len(types) != len(wantNames) {
		t.Fatalf("count: got %d (%v), want %d (%v)", len(types), typeNames(types), len(wantNames), wantNames)
	}
	for i, want := range wantNames {
		if got := TypeName(types[i]); got != want {
			t.Errorf("types[%d]: got %q, want %q", i, got, want)
		}
	}
}

func typeNames(ts []Type) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = TypeName(t)
	}
	return out
}

func TestTupleTypes_FiltersToTuplesOnly(t *testing.T) {
	types := []Type{
		PrimU32,
		TypeString{},
		&TypeList{Elem: PrimU32},
		&TypeTuple{Fields: []Type{PrimU32, PrimU32}},
		&TypeTuple{Fields: []Type{TypeString{}}},
	}
	tuples := TupleTypes(types)
	if len(tuples) != 2 {
		t.Fatalf("count: got %d, want 2", len(tuples))
	}
}

func TestHelperTypes_ExcludesPrimitives(t *testing.T) {
	types := []Type{
		PrimU32,
		TypeString{},
		&TypeList{Elem: PrimU32},
		PrimBool,
		&TypeTuple{Fields: []Type{PrimU32}},
	}
	helpers := HelperTypes(types)
	// Want: TypeString, *TypeList, *TypeTuple — 3 total.
	if len(helpers) != 3 {
		t.Fatalf("count: got %d (%v), want 3", len(helpers), typeNames(helpers))
	}
	for _, h := range helpers {
		if _, isPrim := h.(Prim); isPrim {
			t.Errorf("helper unexpectedly includes Prim %v", h)
		}
	}
}
