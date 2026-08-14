package core

import (
	"context"
	"testing"
)

func TestTypeResource_InstanceAccessor(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	var tr *TypeResource
	inst, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(inst *ComponentInstance) ([]InstanceExport, error) {
			tr = NewTypeResource(inst, nil)
			return []InstanceExport{{Name: "r", Kind: InstanceKindType, Type: tr}}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if got := tr.Instance(); got != inst {
		t.Fatalf("tr.Instance() = %p, want %p", got, inst)
	}
}

// Naming a resource in an instance's exports makes it resolvable
// there; it does not make that instance the definer. An exported
// instance assembled from exports names types defined elsewhere.
func TestTypeResource_ExportDoesNotDefine(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	var tr *TypeResource
	definer, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(inst *ComponentInstance) ([]InstanceExport, error) {
			tr = NewTypeResource(inst, nil)
			return []InstanceExport{{Name: "r", Kind: InstanceKindType, Type: tr}}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewInstance definer: %v", err)
	}

	// A second instance re-exports the same type without defining it.
	reexporter, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(*ComponentInstance) ([]InstanceExport, error) {
			return []InstanceExport{{Name: "r", Kind: InstanceKindType, Type: tr}}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewInstance reexporter: %v", err)
	}

	if got := reexporter.ExportedType("r"); got != tr {
		t.Fatalf("ExportedType(r) = %v, want the re-exported TR to resolve", got)
	}
	if got := tr.Instance(); got != definer {
		t.Fatalf("tr.Instance() = %p, want definer %p — exporting a type must not transfer definition", got, definer)
	}
}

// A BuildExports error must not yield a partially populated instance.
func TestNewInstance_BuildExportsErrorReturnsNoInstance(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	inst, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(*ComponentInstance) ([]InstanceExport, error) {
			return nil, context.Canceled
		},
	})
	if err == nil {
		t.Fatal("NewInstance returned nil error for a failing BuildExports")
	}
	if inst != nil {
		t.Fatalf("NewInstance returned a %p alongside an error, want nil", inst)
	}
}
