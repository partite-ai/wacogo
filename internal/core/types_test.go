package core

import (
	"context"
	"testing"
)

func TestTypeResource_InstanceAccessor(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	tr := NewTypeResource(nil)
	inst, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(*ComponentInstance) ([]InstanceTypeSlot, []InstanceExport, error) {
			return []InstanceTypeSlot{{Name: "r", Type: tr}}, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if got := tr.Instance(); got != inst {
		t.Fatalf("tr.Instance() = %p, want %p", got, inst)
	}
}
