package witgen

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files in genfixtures")

func TestGolden(t *testing.T) {
	cases := []struct {
		name        string
		witPath     string
		world       string
		packageRoot string
	}{
		{
			name:        "Add",
			witPath:     "testdata/wit/add.wit",
			world:       "example:demo/arith",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Greet",
			witPath:     "testdata/wit/greet.wit",
			world:       "example:demo/echohost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Primlists",
			witPath:     "testdata/wit/primlists.wit",
			world:       "example:demo/primlistshost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Sumlist",
			witPath:     "testdata/wit/sumlist.wit",
			world:       "example:demo/summer",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Pair",
			witPath:     "testdata/wit/pair.wit",
			world:       "example:demo/pairhost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Midpoint",
			witPath:     "testdata/wit/midpoint.wit",
			world:       "example:demo/midpointhost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Concat",
			witPath:     "testdata/wit/concat.wit",
			world:       "example:demo/joiner",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Maybe",
			witPath:     "testdata/wit/maybe.wit",
			world:       "example:demo/maybehost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Divide",
			witPath:     "testdata/wit/divide.wit",
			world:       "example:demo/dividehost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Compass",
			witPath:     "testdata/wit/compass.wit",
			world:       "example:demo/compasshost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Acl",
			witPath:     "testdata/wit/acl.wit",
			world:       "example:demo/aclhost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Shape",
			witPath:     "testdata/wit/shape.wit",
			world:       "example:demo/shapehost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Counter",
			witPath:     "testdata/wit/counter.wit",
			world:       "example:demo/counterworld",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "CrossResource",
			witPath:     "testdata/wit/cross_resource.wit",
			world:       "example:demo/crossres",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Compounds",
			witPath:     "testdata/wit/compounds.wit",
			world:       "example:demo/compounds-host",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
		{
			name:        "Docs",
			witPath:     "testdata/wit/docs.wit",
			world:       "example:demo/docshost",
			packageRoot: "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			files, err := generateCached(t, Options{
				WitPath:     c.witPath,
				World:       c.world,
				OutDir:      "testdata/genfixtures",
				PackageRoot: c.packageRoot,
				DryRun:      true,
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			for relPath, content := range files {
				goldenPath := filepath.Join("testdata/genfixtures", relPath)
				if *update {
					if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
						t.Fatalf("mkdir %s: %v", filepath.Dir(goldenPath), err)
					}
					if err := os.WriteFile(goldenPath, content, 0o644); err != nil {
						t.Fatalf("update %s: %v", goldenPath, err)
					}
					continue
				}
				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("read golden %s: %v", goldenPath, err)
				}
				if !bytes.Equal(want, content) {
					t.Errorf("%s differs from golden. Re-run with -update to regenerate.\n--- want ---\n%s\n--- got ---\n%s", relPath, want, content)
				}
			}
		})
	}
}
