package witgen

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenfixturesNoPanic asserts that no panic( appears in any
// generated .go file under testdata/genfixtures, excluding line
// comments, the file-level "DO NOT EDIT" header, and the explicit
// NewInstance deps-validation panics (which signal programmer wiring
// errors, not runtime failures).
func TestGenfixturesNoPanic(t *testing.T) {
	walkErr := filepath.WalkDir("testdata/genfixtures", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(body), "\n") {
			// Strip line comments before scanning to avoid false positives
			// on '// ...panic(...)...' doc strings.
			code := line
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}
			if !strings.Contains(code, "panic(") {
				continue
			}
			// Allow explicit NewInstance deps-validation panics.
			if strings.Contains(code, "NewInstance: deps") {
				continue
			}
			t.Errorf("%s:%d: contains panic(: %q", path, i+1, line)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

// TestGenfixturesNoContextBackground asserts that no
// context.Background() literal appears in any generated file.
func TestGenfixturesNoContextBackground(t *testing.T) {
	walkErr := filepath.WalkDir("testdata/genfixtures", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(body), "\n") {
			code := line
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}
			if strings.Contains(code, "context.Background()") {
				t.Errorf("%s:%d: contains context.Background(): %q", path, i+1, line)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}
