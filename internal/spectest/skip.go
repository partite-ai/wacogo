// Package spectest holds shared helpers for running the WebAssembly
// component-model spec test suite vendored under testdata/spec/.
//
// The package is intentionally small. Test files in wasmparser/ and
// internal/core/ load the same skip manifest and walk subsets of the same
// mirror, so coverage is uniformly auditable.
package spectest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkipManifest records which spec-mirror files or per-file assertions are
// excluded from coverage, plus a human reason for each exclusion.
//
// Paths are slash-separated and rooted at the spec mirror (e.g.
// "async/cancellable.wast"). File entries support a single trailing-segment
// glob ("async/*.wast"); assertion entries must name an exact file.
type SkipManifest struct {
	Files      []FileSkip      `json:"files"`
	Assertions []AssertionSkip `json:"assertions"`

	// matched tracks which entries actually matched something on disk so
	// the drift check can report stale manifest entries.
	matchedFiles      map[int]bool
	matchedAssertions map[int]map[int]bool // entry index -> set of line numbers
}

// FileSkip excludes one or more whole files from the run.
type FileSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// AssertionSkip excludes specific assertions inside one file. Lines are
// 1-based and refer to the upstream .wast source line of the assertion.
type AssertionSkip struct {
	Path   string `json:"path"`
	Lines  []int  `json:"lines"`
	Reason string `json:"reason"`
}

// LoadSkipManifest reads and validates a skip manifest from disk.
func LoadSkipManifest(path string) (*SkipManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skip manifest: %w", err)
	}
	var m SkipManifest
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode skip manifest %s: %w", path, err)
	}
	for i, fs := range m.Files {
		if fs.Path == "" {
			return nil, fmt.Errorf("%s: files[%d]: empty path", path, i)
		}
		if fs.Reason == "" {
			return nil, fmt.Errorf("%s: files[%d] (%s): empty reason", path, i, fs.Path)
		}
	}
	for i, as := range m.Assertions {
		if as.Path == "" {
			return nil, fmt.Errorf("%s: assertions[%d]: empty path", path, i)
		}
		if as.Reason == "" {
			return nil, fmt.Errorf("%s: assertions[%d] (%s): empty reason", path, i, as.Path)
		}
		if len(as.Lines) == 0 {
			return nil, fmt.Errorf("%s: assertions[%d] (%s): empty lines", path, i, as.Path)
		}
		if strings.ContainsAny(as.Path, "*?[") {
			return nil, fmt.Errorf("%s: assertions[%d] (%s): path may not contain glob metacharacters", path, i, as.Path)
		}
	}
	m.matchedFiles = make(map[int]bool)
	m.matchedAssertions = make(map[int]map[int]bool)
	return &m, nil
}

// FileSkip returns the reason this file is skipped wholesale, or "" if it
// is not skipped.
func (m *SkipManifest) FileSkip(rel string) string {
	rel = filepath.ToSlash(rel)
	for i, fs := range m.Files {
		ok, err := filepath.Match(fs.Path, rel)
		if err == nil && ok {
			m.matchedFiles[i] = true
			return fs.Reason
		}
	}
	return ""
}

// AssertionSkip returns the reason this assertion (file + 1-based source
// line) is skipped, or "" if it is not skipped.
func (m *SkipManifest) AssertionSkip(rel string, line int) string {
	rel = filepath.ToSlash(rel)
	for i, as := range m.Assertions {
		if as.Path != rel {
			continue
		}
		for _, l := range as.Lines {
			if l == line {
				if m.matchedAssertions[i] == nil {
					m.matchedAssertions[i] = make(map[int]bool)
				}
				m.matchedAssertions[i][l] = true
				return as.Reason
			}
		}
	}
	return ""
}

// UnusedFileEntries returns file-level manifest entries that never matched
// anything on disk. File entries support glob matching, so a no-match
// indicates the underlying file was deleted/renamed upstream.
func (m *SkipManifest) UnusedFileEntries() []string {
	var out []string
	for i, fs := range m.Files {
		if !m.matchedFiles[i] {
			out = append(out, fmt.Sprintf("files[%d] %q (%s) — no matching file in mirror", i, fs.Path, fs.Reason))
		}
	}
	sort.Strings(out)
	return out
}

// UnusedAssertionEntries returns assertion-level manifest entries whose
// (path, line) pair never matched anything during the run. Stale lines
// usually mean upstream reordered the .wast.
func (m *SkipManifest) UnusedAssertionEntries() []string {
	var out []string
	for i, as := range m.Assertions {
		matched := m.matchedAssertions[i]
		var missing []int
		for _, l := range as.Lines {
			if !matched[l] {
				missing = append(missing, l)
			}
		}
		if len(missing) > 0 {
			out = append(out, fmt.Sprintf("assertions[%d] %q lines %v (%s) — no matching assertion in mirror", i, as.Path, missing, as.Reason))
		}
	}
	sort.Strings(out)
	return out
}
