package spectest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WastFile names one upstream .wast file inside the mirror.
type WastFile struct {
	// Rel is the slash-separated path relative to the spec-mirror root
	// (e.g. "wasm-tools/alias.wast"). Stable across machines.
	Rel string
	// Abs is the absolute path on disk.
	Abs string
}

// ListWast lists all .wast files in the given subdirectories of the spec
// mirror, returned in deterministic order.
//
// specRoot is the path to testdata/spec (typically reached via "../../testdata/spec"
// from a test file). subdirs are slash-separated mirror-relative subdirectories
// like "wasm-tools" or "wasmtime"; pass nil to walk everything.
func ListWast(specRoot string, subdirs []string) ([]WastFile, error) {
	if len(subdirs) == 0 {
		entries, err := os.ReadDir(specRoot)
		if err != nil {
			return nil, fmt.Errorf("read spec root %s: %w", specRoot, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				subdirs = append(subdirs, e.Name())
			}
		}
	}

	var out []WastFile
	for _, sub := range subdirs {
		base := filepath.Join(specRoot, sub)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".wast") {
				return nil
			}
			rel, err := filepath.Rel(specRoot, path)
			if err != nil {
				return err
			}
			out = append(out, WastFile{
				Rel: filepath.ToSlash(rel),
				Abs: path,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", base, err)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}
