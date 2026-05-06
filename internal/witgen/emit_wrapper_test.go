package witgen

import (
	"strings"
	"testing"
)

func TestGenerate_EmitsWrapperFile(t *testing.T) {
	out, err := generateCached(t, Options{
		WitPath:     "testdata/wit/counter.wit",
		World:       "example:demo/counterworld",
		OutDir:      t.TempDir(),
		PackageRoot: "example.com/test",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var found string
	for path := range out {
		if strings.HasSuffix(path, ".wrap.go") {
			found = path
			break
		}
	}
	if found == "" {
		var paths []string
		for p := range out {
			paths = append(paths, p)
		}
		t.Fatalf("no .wrap.go file emitted; got paths: %v", paths)
	}
	content := string(out[found])
	if !strings.Contains(content, "WrapInstance") {
		t.Errorf("missing WrapInstance in %s:\n%s", found, content)
	}
	if !strings.Contains(content, "counterhostWrapper") {
		t.Errorf("missing counterhostWrapper in %s:\n%s", found, content)
	}
	if !strings.Contains(content, "Counterhost") {
		t.Errorf("missing Counterhost interface reference in %s:\n%s", found, content)
	}
}

func TestEmitWrapperFile_CounterInterface(t *testing.T) {
	iface := &Interface{
		Namespace: "example",
		Package:   "demo",
		Name:      "counterhost",
		GoPackage: "counterhost",
		GoName:    "Counterhost",
		ImplName:  "CounterhostImpl",
		WrapName:  "Counterhost",
		Resources: []*TypeResource{
			{
				Name:     "counter",
				GoName:   "Counter",
				ImplName: "CounterImpl",
				WrapName: "Counter",
				Ctor: &ResourceCtor{
					Params: []*Param{
						{GoName: "initial", Type: PrimU32},
					},
				},
				Methods: []ResourceMethod{
					{Name: "current", GoName: "Current", Result: PrimU32},
					{Name: "increment", GoName: "Increment"},
				},
			},
		},
	}

	src, err := emitWrapperFile(iface, "counter.wit")
	if err != nil {
		t.Fatalf("emitWrapperFile: %v", err)
	}
	text := string(src)

	checks := []struct {
		want string
		desc string
	}{
		{"counterhostWrapper", "wrapper struct name"},
		{"fnNewCounter", "ctor field name"},
		{"fnCounterCurrent", "method field name"},
		{"fnCounterIncrement", "method field name"},
		{"[constructor]counter", "ctor WIT export name"},
		{"[method]counter.current", "method WIT export name"},
		{"func WrapInstance(caller *host.ComponentInstance, callee *wacogo.ComponentInstance) Counterhost", "WrapInstance signature"},
		{"func (w *counterhostWrapper) NewCounter(ctx context.Context, initial uint32) (*CounterHandle, error)", "ctor method signature"},
		{"CallRaw", "real CallRaw body"},
		{"func(caller, callee *host.CallContext, stack []uint64)", "inner closure signature"},
	}
	for _, c := range checks {
		if !strings.Contains(text, c.want) {
			t.Errorf("missing %s (%q) in output:\n%s", c.desc, c.want, text)
		}
	}
}
