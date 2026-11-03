package witgen

import (
	"path/filepath"
	"strings"
	"testing"
)

// A component importing an interface can name its primitive aliases, so
// the host factory must export each one while uses stay inline.
func TestGenerate_ExportsPrimitiveAliases(t *testing.T) {
	out, err := generateCached(t, Options{
		WitPath:     "testdata/wit/primitive_alias.wit",
		World:       "example:demo/idshost",
		OutDir:      t.TempDir(),
		PackageRoot: "example.com/test",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for filename, wants := range map[string][]string{
		"ids.bind.go": {
			`b_.AddType("occurrence-id", host.String)`,
			`b_.AddType("count", host.U32)`,
			`b_.AddType("copied-id", host.String)`,
			`{Name: "id", Type: host.String}`,
			`{Name: "n", Type: host.U32}`,
		},
		"consumer.bind.go": {
			`b_.AddType("imported-id", host.String)`,
			`b_.AddType("another-id", host.String)`,
		},
	} {
		t.Run(filename, func(t *testing.T) {
			var content string
			for path, data := range out {
				if filepath.Base(path) == filename {
					content = string(data)
				}
			}
			if content == "" {
				t.Fatalf("no %s file emitted", filename)
			}
			for _, want := range wants {
				if !strings.Contains(content, want) {
					t.Errorf("missing %q in %s", want, filename)
				}
			}
		})
	}
}
