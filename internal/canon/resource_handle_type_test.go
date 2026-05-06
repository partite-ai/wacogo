package canon

import (
	"testing"
)

func TestStubHandle_Type(t *testing.T) {
	rt := &testResourceType{name: "R"}
	tbl := newStubResourceTable()
	h := tbl.IssueOwn(rt, 7)
	if h.Type() != rt {
		t.Fatalf("Type() = %v, want %v", h.Type(), rt)
	}
	val := newValOwnHandleEmpty()
	if _, err := h.TransferOwn(val); err != nil {
		t.Fatalf("TransferOwn into val: %v", err)
	}
	if val.Type() != rt {
		t.Fatalf("val Type() = %v, want %v", val.Type(), rt)
	}
}
