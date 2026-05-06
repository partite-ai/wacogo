package canon

import "testing"

func TestCallBinding_Construction(t *testing.T) {
	cb := NewCallBinding(nil, nil, Callee{CallSide: CallSide{Instance: &testInstance{}}})
	if cb == nil {
		t.Fatal("NewCallBinding returned nil")
	}
	if cb.plan == nil {
		t.Fatal("plan not compiled")
	}
}

func TestCallBinding_CalleeGetter(t *testing.T) {
	inst := &testInstance{name: "inst"}
	want := Callee{CallSide: CallSide{Instance: inst, StringEncoding: EncUTF8}}
	cb := NewCallBinding(nil, nil, want)
	got := cb.Callee()
	if got.Instance != want.Instance || got.StringEncoding != want.StringEncoding {
		t.Fatalf("Callee() = %+v, want %+v", got, want)
	}
}
