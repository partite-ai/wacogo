package core

import (
	"context"
	"testing"
)

func TestNewInstance_MinimalShape(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	spec := &InstanceSpec{}
	inst, err := NewInstance(e, spec)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if inst == nil {
		t.Fatal("want non-nil *ComponentInstance")
	}
	if inst.engine != e {
		t.Fatal("want engine wired")
	}
	if inst.exports == nil {
		t.Fatal("want non-nil exports map (even when empty)")
	}
}

func TestNewInstance_CloseOrdersModules(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	inst, err := NewInstance(e, &InstanceSpec{
		Modules: nil,
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	if err := inst.Close(ctx); err != nil {
		t.Fatalf("Close on empty Modules: %v", err)
	}
}

func TestNewInstance_NestedInstanceExport(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	child, err := NewInstance(e, &InstanceSpec{})
	if err != nil {
		t.Fatalf("child NewInstance: %v", err)
	}

	parent, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(*ComponentInstance) ([]InstanceExport, error) {
			return []InstanceExport{
				{Name: "nested", Kind: InstanceKindInstance, Instance: child},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("parent NewInstance: %v", err)
	}

	got := parent.ExportedInstance("nested")
	if got != child {
		t.Fatalf("ExportedInstance(\"nested\") = %p, want %p", got, child)
	}
}

func TestNewInstance_CoreModuleExport(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	cm, err := e.runtime.CompileModule(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("CompileModule: %v", err)
	}
	mod := &CompiledModule{module: cm}

	parent, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(*ComponentInstance) ([]InstanceExport, error) {
			return []InstanceExport{
				{Name: "m", Kind: InstanceKindCoreModule, CompiledModule: mod},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("parent NewInstance: %v", err)
	}

	got := parent.ExportedModule("m")
	if got != mod {
		t.Fatalf("ExportedModule(\"m\") = %p, want %p", got, mod)
	}
}
