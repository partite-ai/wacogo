package witgen

import (
	"sync"
	"testing"

	"go.bytecodealliance.org/wit"
)

// loadCache memoizes Load results across a test process. wit.LoadWIT
// boots wasm-tools inside wazero (~25–50ms each call); the witgen tests
// hit a small set of paths repeatedly, so a path → *wit.Resolve cache
// drops a sizeable chunk off the suite. LowerWorld and the emit phase
// don't mutate *Resolve, so sharing is safe.
var loadCache sync.Map // map[string]loadCacheEntry

type loadCacheEntry struct {
	once sync.Once
	res  *wit.Resolve
	err  error
}

func loadCached(t testing.TB, path string) *wit.Resolve {
	t.Helper()
	v, _ := loadCache.LoadOrStore(path, &loadCacheEntry{})
	e := v.(*loadCacheEntry)
	e.once.Do(func() { e.res, e.err = Load(path) })
	if e.err != nil {
		t.Fatalf("Load %s: %v", path, e.err)
	}
	return e.res
}

// generateCached runs Generate using a cached *wit.Resolve. opts.WitPath
// still picks the file; the cache key is the path.
func generateCached(t testing.TB, opts Options) (map[string][]byte, error) {
	t.Helper()
	if opts.WitPath == "" || opts.World == "" || opts.OutDir == "" || opts.PackageRoot == "" {
		t.Fatalf("generateCached: WitPath, World, OutDir, PackageRoot are all required")
	}
	res := loadCached(t, opts.WitPath)
	return generateFromResolve(res, opts)
}
