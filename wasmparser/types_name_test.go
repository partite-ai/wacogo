package wasmparser

import "testing"

func TestIsKebabCase(t *testing.T) {
	valid := []string{
		"foo",
		"foo-bar",
		"a",
		"my-component",
		"x86",
		"hello-world",
		"abc-123",
		"foo-bar-baz",
		"FOO",       // all-uppercase single word is valid
		"X",         // single uppercase letter is valid
		"FOO-BAR",   // all-uppercase words are valid
	}

	for _, s := range valid {
		if !IsKebabCase(s) {
			t.Errorf("expected %q to be valid kebab-case", s)
		}
	}

	invalid := []string{
		"",
		"-foo",
		"foo-",
		"foo--bar",
		"Foo",       // mixed case within word
		"foo_bar",
		"foo bar",
		"123",       // starts with digit
		"foo-Bar",   // mixed case within "Bar" word
		"-",
		"fOo",      // mixed case within word
	}
	for _, s := range invalid {
		if IsKebabCase(s) {
			t.Errorf("expected %q to be invalid kebab-case", s)
		}
	}
}

func TestIsKebabCaseDigitWords(t *testing.T) {
	// A trailing all-digit word after a hyphen is valid.
	if !IsKebabCase("foo-123") {
		t.Error("expected foo-123 to be valid kebab-case")
	}
	// But a leading all-digit word is not.
	if IsKebabCase("123-foo") {
		t.Error("expected 123-foo to be invalid kebab-case")
	}
}

func TestParseComponentName(t *testing.T) {
	tests := []struct {
		raw      string
		wantKind ComponentNameKind
		wantRes  string
		wantMeth string
		wantErr  bool
	}{
		// Plain labels
		{"foo", ComponentNameLabel, "", "", false},
		{"my-component", ComponentNameLabel, "", "", false},

		// Interface names
		{"wasi:http/types", ComponentNameInterface, "", "", false},
		{"wasi:http/types@2.0.0", ComponentNameInterface, "", "", false},
		{"wasi:clocks/wall-clock", ComponentNameInterface, "", "", false},

		// Constructor
		{"[constructor]my-resource", ComponentNameConstructor, "my-resource", "", false},

		// Method
		{"[method]my-resource.do-thing", ComponentNameMethod, "my-resource", "do-thing", false},

		// Static
		{"[static]my-resource.create", ComponentNameStatic, "my-resource", "create", false},

		// Dependency names
		{"locked-dep=foo:bar/baz@1.0.0", ComponentNameDependency, "", "", false},
		{"unlocked-dep=foo:bar/baz@1.0.0", ComponentNameDependency, "", "", false},

		// URL
		{"url=https://example.com/foo.wasm", ComponentNameURL, "", "", false},

		// Hash / integrity
		{"integrity=sha256-abc123", ComponentNameHash, "", "", false},

		// Errors
		{"", ComponentNameLabel, "", "", true},
		{"[constructor]My-Resource", ComponentNameLabel, "", "", true},  // uppercase invalid
		{"[method]res", ComponentNameLabel, "", "", true},               // missing dot
		{"[unknown]foo", ComponentNameLabel, "", "", true},              // bad bracket prefix
	}

	for _, tt := range tests {
		cn, err := ParseComponentName(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseComponentName(%q): expected error, got none", tt.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseComponentName(%q): unexpected error: %v", tt.raw, err)
			continue
		}
		if cn.Kind != tt.wantKind {
			t.Errorf("ParseComponentName(%q): kind = %d, want %d", tt.raw, cn.Kind, tt.wantKind)
		}
		if cn.Resource != tt.wantRes {
			t.Errorf("ParseComponentName(%q): resource = %q, want %q", tt.raw, cn.Resource, tt.wantRes)
		}
		if cn.Method != tt.wantMeth {
			t.Errorf("ParseComponentName(%q): method = %q, want %q", tt.raw, cn.Method, tt.wantMeth)
		}
		if cn.Raw != tt.raw {
			t.Errorf("ParseComponentName(%q): raw = %q, want %q", tt.raw, cn.Raw, tt.raw)
		}
	}
}
