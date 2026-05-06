package spectest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is one .wast file ready to be exercised.
type File struct {
	// Source distinguishes spec-mirror from local-only.
	Source Source
	// Rel is the slash-separated path relative to the Source root, e.g.
	// "wasm-tools/alias.wast" or "parser/alias-extras.wast". Used as the
	// key in the skip manifest.
	Rel string
	// Source path on disk to the .wast file (informational).
	WastPath string
	// FixtureJSON is the absolute path to the compiled JSON fixture.
	// Empty when SkipReason is non-empty.
	FixtureJSON string
	// FixtureWasmDir is the directory of compiled .wasm helpers for the
	// fixture. Empty when SkipReason is non-empty.
	FixtureWasmDir string
	// SkipReason is non-empty when the manifest excludes this whole file.
	SkipReason string
}

// Plan walks every .wast file under testdata/spec/ and testdata/local/,
// applies the spec-mirror manifest to determine whole-file skips, and
// returns the files in deterministic (Source, Rel) order. The directory
// layout itself is the source of truth for what runs — adding a new
// upstream subdirectory automatically opts it in (or it can be opted out
// via the manifest).
//
// Files in SourceLocal are NEVER skipped by the manifest — local tests are
// authored by us and represent intentional behavior, not upstream coverage.
func (l *Layout) Plan(manifest *SkipManifest) ([]File, error) {
	sources := []Source{SourceSpec, SourceLocal}
	var out []File
	for _, src := range sources {
		root := l.SourceRoot(src)
		genRoot := l.GeneratedRoot(src)
		wasts, err := ListWast(root, nil)
		if err != nil {
			return nil, err
		}
		for _, w := range wasts {
			f := File{
				Source:   src,
				Rel:      w.Rel,
				WastPath: w.Abs,
			}
			if src == SourceSpec && manifest != nil {
				if reason := manifest.FileSkip(w.Rel); reason != "" {
					f.SkipReason = reason
					out = append(out, f)
					continue
				}
			}
			stem := strings.TrimSuffix(w.Rel, ".wast")
			f.FixtureJSON = filepath.Join(genRoot, stem+".json")
			f.FixtureWasmDir = filepath.Join(genRoot, stem)
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Rel < out[j].Rel
	})
	return out, nil
}

// CheckFixturesExist confirms each non-skipped file has a JSON fixture on
// disk. Returns a non-nil error pointing the user at `go generate` when any
// fixture is missing.
func CheckFixturesExist(files []File) error {
	var missing []string
	for _, f := range files {
		if f.SkipReason != "" {
			continue
		}
		if _, err := os.Stat(f.FixtureJSON); err != nil {
			missing = append(missing, f.FixtureJSON)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing fixtures (run 'go generate ./testdata'):\n  %s", strings.Join(missing, "\n  "))
	}
	return nil
}
