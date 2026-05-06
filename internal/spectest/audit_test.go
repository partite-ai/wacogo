package spectest

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSpecAuditNoStaleManifestEntries confirms every manifest entry
// matched at least one file or assertion. Stale entries indicate the
// upstream renamed/removed/reordered something — bump the SHA, then fix
// or delete the entry.
func TestSpecAuditNoStaleManifestEntries(t *testing.T) {
	layout, err := Discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	manifest, err := LoadSkipManifest(layout.ManifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	// Drive the manifest against every spec file and every assertion line
	// from the compiled fixtures. We don't have direct access to assertion
	// lines here, so the test verifies file-level entries only — the
	// per-package runners cover assertion-level liveness because they
	// invoke AssertionSkip during normal runs.
	wasts, err := ListWast(layout.SpecRoot, nil)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	for _, w := range wasts {
		_ = manifest.FileSkip(w.Rel)
	}

	unused := manifest.UnusedFileEntries()
	if len(unused) > 0 {
		t.Errorf("stale manifest file entries (no matching file in mirror — re-sync upstream or remove the entry):\n  %s",
			strings.Join(unused, "\n  "))
	}
}

// TestSpecAuditUpstreamPinPresent confirms UPSTREAM.txt exists and names a
// commit. Without it, syncspec cannot run.
func TestSpecAuditUpstreamPinPresent(t *testing.T) {
	layout, err := Discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	upstream := filepath.Join(layout.SpecRoot, "UPSTREAM.txt")
	commit, err := readCommitForTest(upstream)
	if err != nil {
		t.Fatalf("read UPSTREAM.txt: %v", err)
	}
	if len(commit) < 7 {
		t.Errorf("UPSTREAM.txt commit %q looks too short", commit)
	}
}
