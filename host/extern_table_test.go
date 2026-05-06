package host

import (
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

func TestExternTable_PutLookupRelease_Basic(t *testing.T) {
	var et externTable
	type widget struct{ id int }
	w := &widget{id: 11}

	eh := et.put(w)
	if eh == 0 {
		t.Fatal("put returned ExternHandle 0; should be 1-indexed")
	}
	got, ok := et.lookup(eh)
	if !ok || got != w {
		t.Fatalf("lookup: got (%v, %v), want (%v, true)", got, ok, w)
	}
	freed, ok := et.release(eh)
	if !ok || freed != w {
		t.Fatalf("release returned (%v, %v), want (%v, true)", freed, ok, w)
	}
	if got, ok := et.lookup(eh); ok {
		t.Fatalf("lookup after release should return ok=false, got %v", got)
	}
}

func TestExternTable_FreeListReuse(t *testing.T) {
	var et externTable
	a := et.put("a")
	b := et.put("b")
	et.release(a)
	c := et.put("c")
	if c != a {
		t.Fatalf("put after release should reuse: got handle %d, want %d", c, a)
	}
	got, ok := et.lookup(b)
	if !ok || got != "b" {
		t.Fatalf("lookup(b): got (%v, %v), want (%q, true)", got, ok, "b")
	}
}

func TestExternTable_LookupUnknownReturnsFalse(t *testing.T) {
	var et externTable
	if _, ok := et.lookup(ExternHandle(999)); ok {
		t.Fatal("lookup(unknown): ok=true, want false")
	}
}

func TestExternTable_SatisfiesCoreInterface(t *testing.T) {
	var _ core.ExternTable = (*externTable)(nil)
	tab := &externTable{}
	eh := tab.put("hello")
	v, ok := tab.Lookup(uint32(eh))
	if !ok || v.(string) != "hello" {
		t.Fatalf("Lookup(%d) = (%v, %v), want (\"hello\", true)", eh, v, ok)
	}
	if _, ok := tab.Lookup(0); ok {
		t.Fatal("Lookup(0) ok=true, want false")
	}
	if _, ok := tab.Lookup(uint32(eh) + 99); ok {
		t.Fatal("Lookup(out-of-range) ok=true, want false")
	}
}
