package witgen

import (
	"testing"
)

func TestLoad_AddFixture(t *testing.T) {
	res, err := Load("testdata/wit/add.wit")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Worlds) != 1 {
		t.Fatalf("Worlds: got %d, want 1", len(res.Worlds))
	}
	w := res.Worlds[0]
	if w == nil {
		t.Fatal("Worlds[0] is nil")
	}
	// World.Name is a string field directly
	if got := w.Name; got != "arith" {
		t.Fatalf("World name: got %q, want %q", got, "arith")
	}
}
