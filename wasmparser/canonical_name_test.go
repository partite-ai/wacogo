package wasmparser

import "testing"

func TestCanonicalizeImportName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// No version suffix.
		{"wasi:io/error", "wasi:io/error"},
		{"plain", "plain"},
		{"h", "h"},

		// 0.x.y → 0.x (minor>0 split).
		{"wasi:io/error@0.2.3", "wasi:io/error@0.2"},
		{"wasi:io/error@0.2.8", "wasi:io/error@0.2"},
		{"wasi:cli/run@0.2.0", "wasi:cli/run@0.2"},

		// 1.x.y → 1 (major>0 split).
		{"foo:bar/baz@1.2.3", "foo:bar/baz@1"},
		{"foo:bar/baz@2.0.0", "foo:bar/baz@2"},

		// 0.0.x → 0.0.x (patch split).
		{"a:b/c@0.0.1", "a:b/c@0.0.1"},

		// Prerelease/build suffix is stripped along with the trailing version components.
		{"foo:bar/baz@0.2.6-rc.1", "foo:bar/baz@0.2"},
		{"foo:bar/baz@1.2.3+build.5", "foo:bar/baz@1"},

		// Already canonical — idempotent.
		{"wasi:io/error@0.2", "wasi:io/error@0.2"},
		{"foo:bar/baz@1", "foo:bar/baz@1"},
		{"a:b/c@0.0.1-alpha", "a:b/c@0.0.1"},

		// Non-semver-looking suffixes left alone.
		{"foo@bar", "foo@bar"},
		{"foo@", "foo@"},
		{"foo@1.2.3.4", "foo@1.2.3.4"},
	}
	for _, tc := range cases {
		got := CanonicalizeImportName(tc.in)
		if got != tc.want {
			t.Errorf("CanonicalizeImportName(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// Idempotent.
		if again := CanonicalizeImportName(got); again != got {
			t.Errorf("CanonicalizeImportName not idempotent for %q: %q → %q", tc.in, got, again)
		}
	}
}
