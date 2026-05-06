package witgen

import (
	"strings"
	"testing"
)

func TestGenerate_AddDryRun(t *testing.T) {
	files, err := generateCached(t, Options{
		WitPath:     "testdata/wit/add.wit",
		World:       "example:demo/arith",
		OutDir:      "/dev/null",
		PackageRoot: "example.test/gen",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("file count: got %d, want 3", len(files))
	}
	var ifacePath, bindPath, wrapPath string
	for p := range files {
		if strings.HasSuffix(p, "calc.go") {
			ifacePath = p
		}
		if strings.HasSuffix(p, "calc.bind.go") {
			bindPath = p
		}
		if strings.HasSuffix(p, "calc.wrap.go") {
			wrapPath = p
		}
	}
	if ifacePath == "" || bindPath == "" || wrapPath == "" {
		t.Fatalf("expected calc.go, calc.bind.go, and calc.wrap.go, got: %v", files)
	}
	if !strings.Contains(ifacePath, "example/demo/calc/") {
		t.Errorf("iface path %q missing namespace/package/iface segments", ifacePath)
	}
}
