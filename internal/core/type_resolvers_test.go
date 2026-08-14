package core

import "testing"

// makeTestCtx constructs a resolver context with a pre-populated type index
// space for resolver unit tests. No engine, no component; the resolvers only
// read from the instantiation state.
func makeTestCtx(types []Type) *resolverCtx {
	return &resolverCtx{state: &instantiationState{types: types}}
}

func TestListResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeU32{}})
	r := listResolver{elem: indexResolver{idx: 0}}
	got := r.resolve(rc)
	list, ok := got.(TypeList)
	if !ok {
		t.Fatalf("want TypeList, got %T", got)
	}
	if _, ok := list.Elem.(TypeU32); !ok {
		t.Fatalf("want inner TypeU32, got %T", list.Elem)
	}
}

func TestListResolverInlinePrimitive(t *testing.T) {
	// Confirms that a compound resolver's child can be a primitiveResolver
	// directly — the loader uses this shape for inline ComponentValType
	// primitives that never occupy a TypeID slot.
	r := listResolver{elem: primitiveResolver{t: TypeU32{}}}
	got := r.resolve(&resolverCtx{}).(TypeList)
	if _, ok := got.Elem.(TypeU32); !ok {
		t.Fatalf("want TypeU32 inner, got %T", got.Elem)
	}
}

func TestRecordResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeBool{}, TypeString{}})
	r := recordResolver{fields: []recordResolverField{
		{name: "flag", t: indexResolver{idx: 0}},
		{name: "label", t: indexResolver{idx: 1}},
	}}
	got := r.resolve(rc).(TypeRecord)
	if len(got.Fields) != 2 || got.Fields[0].Name != "flag" || got.Fields[1].Name != "label" {
		t.Fatalf("unexpected fields: %+v", got.Fields)
	}
}

func TestTupleResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeU8{}, TypeU16{}})
	r := tupleResolver{elems: []typeResolver{indexResolver{idx: 0}, indexResolver{idx: 1}}}
	got := r.resolve(rc).(TypeTuple)
	if len(got.Types) != 2 {
		t.Fatalf("want 2 types, got %d", len(got.Types))
	}
}

func TestVariantResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeU32{}})
	r := variantResolver{cases: []variantResolverCase{
		{name: "a", payload: indexResolver{idx: 0}, hasPayload: true},
		{name: "b"},
	}}
	got := r.resolve(rc).(TypeVariant)
	if len(got.Cases) != 2 || got.Cases[1].Payload != nil {
		t.Fatalf("unexpected cases: %+v", got.Cases)
	}
}

func TestEnumResolver(t *testing.T) {
	r := enumResolver{cases: []string{"red", "green"}}
	got := r.resolve(&resolverCtx{}).(TypeEnum)
	if len(got.Cases) != 2 {
		t.Fatalf("want 2 cases")
	}
}

func TestOptionResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeU32{}})
	got := optionResolver{inner: indexResolver{idx: 0}}.resolve(rc).(TypeOption)
	if _, ok := got.Inner.(TypeU32); !ok {
		t.Fatalf("want TypeU32 inner")
	}
}

func TestResultResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeU32{}, TypeString{}})
	r := resultResolver{ok: indexResolver{idx: 0}, hasOk: true, err: indexResolver{idx: 1}, hasErr: true}
	got := r.resolve(rc).(TypeResult)
	if got.Ok == nil || got.Err == nil {
		t.Fatalf("ok and err should both be populated")
	}
}

func TestFlagsResolver(t *testing.T) {
	got := flagsResolver{names: []string{"r", "w"}}.resolve(&resolverCtx{}).(TypeFlags)
	if len(got.Names) != 2 {
		t.Fatalf("want 2 names")
	}
}

func TestFuncResolver(t *testing.T) {
	rc := makeTestCtx([]Type{TypeU32{}, TypeString{}})
	r := funcResolver{
		params:  []funcResolverParam{{name: "a", t: indexResolver{idx: 0}}},
		results: []funcResolverParam{{name: "", t: indexResolver{idx: 1}}},
	}
	got, ok := r.resolve(rc).(*FuncType)
	if !ok {
		t.Fatalf("want *FuncType")
	}
	if len(got.Params) != 1 || len(got.Results) != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestOwnResolver(t *testing.T) {
	r := &TypeResource{}
	rc := makeTestCtx([]Type{r})
	got := ownResolver{resource: 0}.resolve(rc).(TypeOwn)
	if got.ResourceType != r {
		t.Fatalf("want %p, got %p", r, got.ResourceType)
	}
}

func TestBorrowResolver(t *testing.T) {
	r := &TypeResource{}
	rc := makeTestCtx([]Type{r})
	got := borrowResolver{resource: 0}.resolve(rc).(TypeBorrow)
	if got.ResourceType != r {
		t.Fatalf("want %p, got %p", r, got.ResourceType)
	}
}

func TestResourceResolverMintsFresh(t *testing.T) {
	inst := &ComponentInstance{}
	rc := makeTestCtx(nil)
	rc.inst = inst
	r := resourceResolver{} // no dtor
	a := r.resolve(rc).(*TypeResource)
	b := r.resolve(rc).(*TypeResource)
	if a == b {
		t.Fatalf("each resolve call must mint a fresh *TypeResource")
	}
	if a.instance != inst {
		t.Fatalf("minted resource must be defined by the instance being built")
	}
}

func TestInstanceImportResolver(t *testing.T) {
	child := &ComponentInstance{
		exports: map[string]exportEntry{"T": {kind: SortType, typ: TypeU32{}}},
	}
	got := instanceImportResolver{instanceIdx: 0, exportName: "T"}.
		resolve(&resolverCtx{state: &instantiationState{
			componentInstances: []*ComponentInstance{child},
		}})
	if _, ok := got.(TypeU32); !ok {
		t.Fatalf("want TypeU32, got %T", got)
	}
}

func TestAliasResolverWalksParent(t *testing.T) {
	gp := &instantiationState{types: []Type{TypeBool{}}}
	p := &instantiationState{parentState: gp}
	c := &instantiationState{parentState: p}
	got := aliasResolver{outerDepth: 2, typeIdx: 0}.resolve(&resolverCtx{state: c})
	if _, ok := got.(TypeBool); !ok {
		t.Fatalf("want TypeBool, got %T", got)
	}
}

func TestInstanceTypeResolverReturnsPlaceholder(t *testing.T) {
	got := instanceTypeResolver{}.resolve(&resolverCtx{})
	if _, ok := got.(*InstanceType); !ok {
		t.Fatalf("want *InstanceType, got %T", got)
	}
}

func TestComponentTypeResolverReturnsPlaceholder(t *testing.T) {
	got := componentTypeResolver{}.resolve(&resolverCtx{})
	if _, ok := got.(*ComponentType); !ok {
		t.Fatalf("want *ComponentType, got %T", got)
	}
}
