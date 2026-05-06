package witgen

import (
	"testing"
)

func TestTypeResource_NameSplit(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_counter.wit")
	pkg, err := LowerWorld(res, "example:demo/countershost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	for _, iface := range pkg.Interfaces {
		for _, r := range iface.Resources {
			if r.WrapName == "" {
				t.Errorf("resource %q: WrapName empty", r.Name)
			}
			if r.ImplName != r.WrapName+"Impl" {
				t.Errorf("resource %q: ImplName %q != WrapName %q + Impl", r.Name, r.ImplName, r.WrapName)
			}
			if r.GoName != r.WrapName {
				t.Errorf("resource %q: GoName %q != WrapName %q", r.Name, r.GoName, r.WrapName)
			}
		}
	}
	// Concrete pin: the counter resource must map to "Counter".
	for _, iface := range pkg.Interfaces {
		for _, r := range iface.Resources {
			if r.Name == "counter" && r.WrapName != "Counter" {
				t.Errorf("counter.WrapName: got %q, want Counter", r.WrapName)
			}
		}
	}
}
