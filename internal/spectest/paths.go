package spectest

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layout points at the canonical testdata/ subdirectories used by the spec
// suite. Tests should construct one via Discover so paths work regardless
// of which package's cwd Go runs them from.
type Layout struct {
	// SpecRoot is testdata/spec — the vendored upstream mirror.
	SpecRoot string
	// LocalRoot is testdata/local — local-only test additions.
	LocalRoot string
	// GeneratedSpecRoot is testdata/generated/spec.
	GeneratedSpecRoot string
	// GeneratedLocalRoot is testdata/generated/local.
	GeneratedLocalRoot string
	// ManifestPath is testdata/spec/skip.json.
	ManifestPath string
}

// Discover finds the testdata/ tree by walking parent directories from the
// current working directory until a go.mod file is found. Tests run with
// cwd set to their package directory; this makes the helper work the same
// from wasmparser/ as from internal/core/.
func Discover() (*Layout, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("could not find go.mod above %s", wd)
		}
		dir = parent
	}
	return &Layout{
		SpecRoot:           filepath.Join(dir, "testdata", "spec"),
		LocalRoot:          filepath.Join(dir, "testdata", "local"),
		GeneratedSpecRoot:  filepath.Join(dir, "testdata", "generated", "spec"),
		GeneratedLocalRoot: filepath.Join(dir, "testdata", "generated", "local"),
		ManifestPath:       filepath.Join(dir, "testdata", "spec", "skip.json"),
	}, nil
}

// SourceRoot returns the source-tree root for a given Source value.
func (l *Layout) SourceRoot(s Source) string {
	switch s {
	case SourceSpec:
		return l.SpecRoot
	case SourceLocal:
		return l.LocalRoot
	}
	return ""
}

// GeneratedRoot returns the generated-fixture root for a given Source.
func (l *Layout) GeneratedRoot(s Source) string {
	switch s {
	case SourceSpec:
		return l.GeneratedSpecRoot
	case SourceLocal:
		return l.GeneratedLocalRoot
	}
	return ""
}

// Source distinguishes upstream-mirror tests from local additions.
type Source int

const (
	SourceSpec Source = iota
	SourceLocal
)

func (s Source) String() string {
	switch s {
	case SourceSpec:
		return "spec"
	case SourceLocal:
		return "local"
	}
	return "unknown"
}
