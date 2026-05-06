package core

import (
	"context"
	"testing"
)

type fakeExtTable struct{ entries map[uint32]any }

func (f *fakeExtTable) Lookup(rep uint32) (any, bool) {
	v, ok := f.entries[rep]
	return v, ok
}

func TestExternTable_InterfaceSatisfiable(t *testing.T) {
	var _ ExternTable = (*fakeExtTable)(nil)
	tab := &fakeExtTable{entries: map[uint32]any{1: "hello"}}
	v, ok := tab.Lookup(1)
	if !ok || v.(string) != "hello" {
		t.Fatalf("Lookup(1) = (%v, %v), want (\"hello\", true)", v, ok)
	}
	if _, ok := tab.Lookup(2); ok {
		t.Fatalf("Lookup(2) ok=true, want false")
	}
}

func TestComponentInstance_LookupExtern_NoTable(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	inst, err := NewInstance(e, &InstanceSpec{})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if _, ok := inst.LookupExtern(42); ok {
		t.Fatal("LookupExtern with no table: ok=true, want false")
	}
}

func TestComponentInstance_LookupExtern_FromSpec(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	tab := &fakeExtTable{entries: map[uint32]any{7: "seven"}}
	inst, err := NewInstance(e, &InstanceSpec{ExternTable: tab})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	v, ok := inst.LookupExtern(7)
	if !ok || v.(string) != "seven" {
		t.Fatalf("LookupExtern(7) = (%v, %v), want (\"seven\", true)", v, ok)
	}
}
