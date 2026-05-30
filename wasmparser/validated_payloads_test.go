package wasmparser

import (
	"bytes"
	"io"
	"testing"
)

func TestValidatedComponentExportHandleAccessors(t *testing.T) {
	arena := newTypeArena(DefaultFeatures())
	fid := arena.pushFuncType(FuncTypeDesc{})
	v := ValidatedComponentExport{
		Export: &ComponentExport{},
		et:     ComponentEntityType{Kind: EntityFunc, FuncID: fid},
		arena:  arena,
	}
	if v.FuncType() == nil {
		t.Fatal("FuncType() should return a non-nil handle")
	}
	if v.ModuleType() != nil {
		t.Fatal("ModuleType() should be nil for a func export")
	}
	if v.ComponentType() != nil {
		t.Fatal("ComponentType() should be nil for a func export")
	}
}

func TestValidatedModuleSectionPayloadModuleType(t *testing.T) {
	arena := newTypeArena(DefaultFeatures())
	mid := arena.pushCoreModuleType(CoreModuleTypeDesc{})
	p := &ValidatedModuleSectionPayload{
		raw:        &ModuleSectionPayload{},
		moduleType: mid,
		arena:      arena,
	}
	if p.ModuleType() == nil {
		t.Fatal("ModuleType() should return a non-nil handle")
	}
}

func TestValidatedComponentExportInstanceTypeReturnsNil(t *testing.T) {
	arena := newTypeArena(DefaultFeatures())
	v := ValidatedComponentExport{
		Export: &ComponentExport{},
		et:     ComponentEntityType{Kind: EntityInstance},
		arena:  arena,
	}
	if v.InstanceType() != nil {
		t.Fatal("InstanceType() should return nil (see method comment)")
	}
}

func TestValidatingParserYieldsValidatedModuleSection(t *testing.T) {
	src := `(component
  (core module (func))
)`
	bin := wat2wasm(t, src)
	vp := NewValidatingParser(bytes.NewReader(bin), DefaultFeatures())
	var seen *ValidatedModuleSectionPayload
	for {
		p, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := p.(*ValidatedModuleSectionPayload); ok {
			seen = v
		}
	}
	if seen == nil {
		t.Fatal("expected a ValidatedModuleSectionPayload to be yielded")
	}
	if seen.ModuleType() == nil {
		t.Fatal("expected a ModuleType handle")
	}
}

func TestValidatingParserYieldsValidatedExportSection(t *testing.T) {
	src := `(component
  (core module $m (func (export "f")))
  (core instance $i (instantiate $m))
  (func (export "f") (canon lift (core func $i "f")))
)`
	bin := wat2wasm(t, src)
	vp := NewValidatingParser(bytes.NewReader(bin), DefaultFeatures())
	var seen *ValidatedComponentExportSectionPayload
	for {
		p, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := p.(*ValidatedComponentExportSectionPayload); ok {
			seen = v
		}
	}
	if seen == nil {
		t.Fatal("expected a ValidatedComponentExportSectionPayload to be yielded")
	}
	var saw bool
	for exp, err := range seen.Items() {
		if err != nil {
			t.Fatal(err)
		}
		if exp.Export.Name.Name == "f" {
			if exp.FuncType() == nil {
				t.Fatal("expected FuncType handle for func export")
			}
			saw = true
		}
	}
	if !saw {
		t.Fatal("did not see export f")
	}
}
