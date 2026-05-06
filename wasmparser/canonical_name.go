package wasmparser

import (
	"strconv"
	"strings"
)

// CanonicalizeImportName returns the canonical form of a component-model
// import or export name per the spec's "Canonical Interface Name" rule:
// the trailing "@<semver>" is reduced to its canonversion prefix
// (e.g. "wasi:io/error@0.2.3" → "wasi:io/error@0.2"), so that names
// with the same canonversion match by literal string equality.
//
// Names without an "@version" suffix, names whose suffix is already in
// canonversion form, and names whose suffix is not a recognizable
// semver are returned unchanged. The function is idempotent.
func CanonicalizeImportName(name string) string {
	at := strings.LastIndex(name, "@")
	if at < 0 {
		return name
	}
	canon, ok := canonicalizeVersion(name[at+1:])
	if !ok {
		return name
	}
	return name[:at+1] + canon
}

// canonicalizeVersion splits a semver into its canonversion prefix per
// the rules in the Component Model spec:
//
//   - major > 0: keep "<major>"
//   - else if minor > 0: keep "<major>.<minor>"
//   - else: keep "<major>.<minor>.<patch>"
//
// A version already in canonversion form (1, 2, 3 numeric components
// without any prerelease/build suffix matching the rule above) is
// returned unchanged. Returns ok=false if the string isn't a parsable
// semver-like sequence of dot-separated non-negative integers.
func canonicalizeVersion(ver string) (string, bool) {
	base := ver
	if i := strings.IndexAny(base, "-+"); i >= 0 {
		base = base[:i]
	}
	parts := strings.Split(base, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return "", false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		if p == "" {
			return "", false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return "", false
		}
		nums[i] = n
	}
	if len(parts) < 3 {
		// Already in canonversion form (no prerelease/build to strip).
		if base == ver {
			return ver, true
		}
		return base, true
	}
	switch {
	case nums[0] > 0:
		return parts[0], true
	case nums[1] > 0:
		return parts[0] + "." + parts[1], true
	default:
		return parts[0] + "." + parts[1] + "." + parts[2], true
	}
}
