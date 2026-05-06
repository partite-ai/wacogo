package wasmparser

import "testing"

func TestValidatorHostTypeBuilder_NonNil(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	if b == nil {
		t.Fatal("want non-nil HostTypeBuilder")
	}
}

func TestHostTypeBuilder_AllocResourceID_Unique(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	a := b.AllocResourceID()
	c := b.AllocResourceID()
	if a == c {
		t.Fatalf("want distinct ResourceIDs, got %v twice", a)
	}
}

func TestHostTypeBuilder_PushDefinedType_StoresInArena(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	id := b.PushDefinedType(DefinedTypeDesc{
		Kind:      DefinedKindPrimitive,
		Primitive: PrimU32,
	})
	if int(id) >= len(v.arena.DefinedTypes) {
		t.Fatalf("id %d out of range (arena has %d entries)", id, len(v.arena.DefinedTypes))
	}
	got := v.arena.DefinedTypes[id]
	if got.Kind != DefinedKindPrimitive || got.Primitive != PrimU32 {
		t.Fatalf("got %+v, want primitive u32", got)
	}
}

func TestHostTypeBuilder_PushFuncType_StoresInArena(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	id := b.PushFuncType(FuncTypeDesc{
		Params: []FuncParamDesc{
			{Name: "x", Type: ValTypeDesc{IsPrimitive: true, Primitive: PrimU32}},
		},
	})
	if int(id) >= len(v.arena.FuncTypes) {
		t.Fatal("id out of range")
	}
	got := v.arena.FuncTypes[id]
	if len(got.Params) != 1 || got.Params[0].Name != "x" {
		t.Fatalf("got %+v", got)
	}
}

func TestHostTypeBuilder_PushComponentType_StoresInArena(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	id := b.PushComponentType(ComponentTypeDesc{
		Imports: map[string]ComponentEntityType{},
		Exports: map[string]ComponentEntityType{},
	})
	if int(id) >= len(v.arena.ComponentTypes) {
		t.Fatal("id out of range")
	}
}

func TestHostTypeBuilder_NewComponentTypeHandle(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	id := b.PushComponentType(ComponentTypeDesc{
		Imports: map[string]ComponentEntityType{},
		Exports: map[string]ComponentEntityType{},
	})
	h := b.NewComponentTypeHandle(id)
	if h == nil {
		t.Fatal("want non-nil *ComponentType")
	}
	// Smoke: NewInstance should return a non-nil *InstanceType.
	inst := h.NewInstance()
	if inst == nil {
		t.Fatal("want non-nil *InstanceType")
	}
}

func TestHostTypeBuilder_NewFuncTypeHandle(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()
	id := b.PushFuncType(FuncTypeDesc{})
	h := b.NewFuncTypeHandle(id)
	if h == nil {
		t.Fatal("want non-nil *FuncType")
	}
}

// TestHostTypeBuilder_SynthComponentCheckInstantiation proves a
// ComponentType built by HostTypeBuilder is a valid subtype target
// for another component's EntityInstance import.
func TestHostTypeBuilder_SynthComponentCheckInstantiation(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	b := v.HostTypeBuilder()

	// Host side: a ComponentType exporting one func "hi: func()".
	hostFnID := b.PushFuncType(FuncTypeDesc{})
	hostCompID := b.PushComponentType(ComponentTypeDesc{
		Imports: map[string]ComponentEntityType{},
		Exports: map[string]ComponentEntityType{
			"hi": {Kind: EntityFunc, FuncID: hostFnID},
		},
		ExportOrder: []string{"hi"},
	})
	hostComp := b.NewComponentTypeHandle(hostCompID)
	hostInst := hostComp.NewInstance()

	// Consumer side: one import "h" of instance type matching host.
	consFnID := b.PushFuncType(FuncTypeDesc{})
	consInstID := b.PushInstanceType(InstanceTypeDesc{
		Exports: map[string]ComponentEntityType{
			"hi": {Kind: EntityFunc, FuncID: consFnID},
		},
	})
	consCompID := b.PushComponentType(ComponentTypeDesc{
		Imports: map[string]ComponentEntityType{
			"h": {Kind: EntityInstance, InstID: consInstID},
		},
		ImportOrder: []string{"h"},
		Exports:     map[string]ComponentEntityType{},
	})
	consComp := b.NewComponentTypeHandle(consCompID)

	// Check: supplying the host instance should satisfy consumer's "h".
	if err := consComp.CheckInstantiation(map[string]any{"h": hostInst}); err != nil {
		t.Fatalf("CheckInstantiation: %v", err)
	}
}
