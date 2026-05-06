package witgen

import (
	"strings"
	"testing"
)

func TestValidate_GoNameCollision(t *testing.T) {
	iface := &Interface{
		Name: "foo", GoName: "Foo", GoPackage: "foo",
		Funcs: []*Func{
			{WitName: "get-value", GoName: "GetValue"},
			{WitName: "get_value", GoName: "GetValue"}, // collision
		},
	}
	pkg := &Package{Interfaces: []*Interface{iface}}
	err := Validate(pkg)
	if err == nil {
		t.Fatal("Validate: want error for Go-name collision, got nil")
	}
	if !strings.Contains(err.Error(), "Go-name collision") {
		t.Errorf("error message: got %q, want it to mention 'Go-name collision'", err.Error())
	}
}

func TestValidate_ReservedKeywordParam(t *testing.T) {
	iface := &Interface{
		Name: "foo", GoName: "Foo", GoPackage: "foo",
		Funcs: []*Func{
			{WitName: "do", GoName: "Do", Params: []*Param{
				{GoName: "type", Type: PrimU32}, // "type" is reserved
			}},
		},
	}
	pkg := &Package{Interfaces: []*Interface{iface}}
	err := Validate(pkg)
	if err == nil {
		t.Fatal("Validate: want error for reserved keyword param, got nil")
	}
	if !strings.Contains(err.Error(), "reserved word") {
		t.Errorf("error: got %q, want it to mention reserved word", err.Error())
	}
}

func TestValidate_AnyTypeResultErr(t *testing.T) {
	// The emit code supports any type in the error arm (enum, record, prim, etc.).
	// Validate should accept result<u32, u32> and result<u32, ErrorCode>.
	iface := &Interface{
		Name: "foo", GoName: "Foo", GoPackage: "foo",
		Funcs: []*Func{
			{WitName: "f", GoName: "F",
				Result: &TypeResult{OK: PrimU32, Err: PrimU32}},
		},
	}
	pkg := &Package{Interfaces: []*Interface{iface}}
	if err := Validate(pkg); err != nil {
		t.Fatalf("result<u32, u32> should now be valid: %v", err)
	}
}

func TestValidate_Clean(t *testing.T) {
	iface := &Interface{
		Name: "foo", GoName: "Foo", GoPackage: "foo",
		Funcs: []*Func{
			{WitName: "ok", GoName: "Ok", Params: []*Param{
				{GoName: "x", Type: PrimU32},
			}, Result: PrimU32},
		},
	}
	pkg := &Package{Interfaces: []*Interface{iface}}
	if err := Validate(pkg); err != nil {
		t.Fatalf("clean interface should validate: %v", err)
	}
}

func TestValidate_CrossImportNominalNameCollision(t *testing.T) {
	a := &Interface{Namespace: "n", Package: "p", Name: "a", GoPackage: "a", WrapName: "A", ImplName: "AImpl"}
	b := &Interface{Namespace: "n", Package: "p", Name: "b", GoPackage: "b", WrapName: "B", ImplName: "BImpl"}
	c := &Interface{Namespace: "n", Package: "p", Name: "c", GoPackage: "c", WrapName: "C", ImplName: "CImpl"}

	c.Imports = []ImportRef{
		{Name: "a", GoPackage: "a", Records: []ImportedNominal{{Name: "foo", GoName: "Foo"}}},
		{Name: "b", GoPackage: "b", Records: []ImportedNominal{{Name: "foo", GoName: "Foo"}}},
	}

	pkg := &Package{Interfaces: []*Interface{a, b, c}}
	err := Validate(pkg)
	if err == nil {
		t.Fatal("expected validation error for duplicate WIT name across imports")
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Errorf("error should mention the colliding WIT name: %v", err)
	}
}

// TestValidate_OwnVsImportedNominalCollision asserts a validation error
// when an interface's own nominal shares a WIT name with an imported
// nominal: both would emit b.AddType("foo", ...) in NewFactory and
// the host builder would reject the duplicate.
func TestValidate_OwnVsImportedNominalCollision(t *testing.T) {
	donor := &Interface{Namespace: "n", Package: "p", Name: "donor", GoPackage: "donor", WrapName: "Donor", ImplName: "DonorImpl"}
	user := &Interface{Namespace: "n", Package: "p", Name: "user", GoPackage: "user", WrapName: "User", ImplName: "UserImpl"}
	user.Records = []*TypeRecord{{Name: "foo", GoName: "Foo", OwnerInterface: user}}
	user.Imports = []ImportRef{
		{Name: "donor", GoPackage: "donor", Records: []ImportedNominal{{Name: "foo", GoName: "Foo"}}},
	}

	pkg := &Package{Interfaces: []*Interface{donor, user}}
	err := Validate(pkg)
	if err == nil {
		t.Fatal("expected validation error for own/imported WIT-name collision")
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Errorf("error should mention the colliding WIT name: %v", err)
	}
	if !strings.Contains(err.Error(), "(own)") {
		t.Errorf("error should mention the own-nominal source: %v", err)
	}
}
