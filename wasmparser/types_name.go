package wasmparser

import (
	"fmt"
	"strings"
)

// IsKebabCase reports whether s is a valid kebab-case identifier per the
// Component Model spec (mixed-case allowed, compared case-insensitively).
//
// Rules:
//   - Not empty.
//   - Cannot start or end with '-'.
//   - No consecutive hyphens ("--").
//   - Each word (segment between hyphens) matches [a-zA-Z][a-zA-Z0-9]* or [0-9]+.
//   - The first character of the whole string cannot be a digit.
func IsKebabCase(s string) bool {
	return isKebabCaseInternal(s, true)
}

// IsStrictKebabCase reports whether s is a valid strict (lowercase-only) kebab-case identifier.
// Used for record fields, variant cases, enum tags, function params, flags.
func IsStrictKebabCase(s string) bool {
	return isKebabCaseInternal(s, false)
}

func isKebabCaseInternal(s string, allowUpperCase bool) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	if strings.Contains(s, "--") {
		return false
	}
	if s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for word := range strings.SplitSeq(s, "-") {
		if len(word) == 0 {
			return false
		}
		if !isKebabWord(word, allowUpperCase) {
			return false
		}
	}
	return true
}

func isKebabWord(w string, allowUpperCase bool) bool {
	if w == "" {
		return false
	}
	if w[0] >= '0' && w[0] <= '9' {
		for _, c := range []byte(w) {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	if allowUpperCase {
		// Each word must be consistently cased: all lowercase+digits or all uppercase+digits
		hasLower := false
		hasUpper := false
		for _, c := range []byte(w) {
			if c >= 'a' && c <= 'z' {
				hasLower = true
			} else if c >= 'A' && c <= 'Z' {
				hasUpper = true
			} else if c >= '0' && c <= '9' {
				// digits are OK
			} else {
				return false
			}
		}
		// Must not mix upper and lower in same word
		if hasLower && hasUpper {
			return false
		}
		// Must start with a letter
		if !((w[0] >= 'a' && w[0] <= 'z') || (w[0] >= 'A' && w[0] <= 'Z')) {
			return false
		}
	} else {
		if w[0] < 'a' || w[0] > 'z' {
			return false
		}
		for _, c := range []byte(w[1:]) {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}
	return true
}

// ComponentNameKind identifies the kind of a structured component name.
type ComponentNameKind uint8

const (
	ComponentNameLabel       ComponentNameKind = iota // plain kebab-case label
	ComponentNameConstructor                          // [constructor]resource-name
	ComponentNameMethod                               // [method]resource.method-name
	ComponentNameStatic                               // [static]resource.method-name
	ComponentNameInterface                            // namespace:package/name[@version]
	ComponentNameDependency                           // locked-dep=... or unlocked-dep=...
	ComponentNameURL                                  // url=...
	ComponentNameHash                                 // integrity=...
)

// ComponentName is a parsed structured component name.
type ComponentName struct {
	Raw      string
	Kind     ComponentNameKind
	Resource string // for Constructor/Method/Static: the resource name
	Method   string // for Method/Static: the method name
}

// ParseComponentName parses a raw component name string and returns a ComponentName.
func ParseComponentName(raw string) (ComponentName, error) {
	if raw == "" {
		return ComponentName{}, fmt.Errorf("component name cannot be empty")
	}

	// Bracket-prefixed names: [constructor], [method], [static].
	if strings.HasPrefix(raw, "[") {
		return parseBracketName(raw)
	}

	// Dependency names.
	if strings.HasPrefix(raw, "locked-dep=") || strings.HasPrefix(raw, "unlocked-dep=") {
		return ComponentName{Raw: raw, Kind: ComponentNameDependency}, nil
	}

	// URL.
	if strings.HasPrefix(raw, "url=") {
		return ComponentName{Raw: raw, Kind: ComponentNameURL}, nil
	}

	// Hash / integrity.
	if strings.HasPrefix(raw, "integrity=") {
		return ComponentName{Raw: raw, Kind: ComponentNameHash}, nil
	}

	// Interface name: contains ':' (namespace:package/name[@version]).
	if strings.ContainsRune(raw, ':') {
		return parseInterfaceName(raw)
	}

	// Plain label.
	if !IsKebabCase(raw) {
		return ComponentName{}, fmt.Errorf("invalid kebab-case component name: %q", raw)
	}
	return ComponentName{Raw: raw, Kind: ComponentNameLabel}, nil
}

// parseBracketName handles [constructor], [method], and [static] names.
func parseBracketName(raw string) (ComponentName, error) {
	var kind ComponentNameKind
	var prefix string

	switch {
	case strings.HasPrefix(raw, "[constructor]"):
		kind = ComponentNameConstructor
		prefix = "[constructor]"
	case strings.HasPrefix(raw, "[method]"):
		kind = ComponentNameMethod
		prefix = "[method]"
	case strings.HasPrefix(raw, "[static]"):
		kind = ComponentNameStatic
		prefix = "[static]"
	default:
		return ComponentName{}, fmt.Errorf("unknown bracket prefix in component name: %q", raw)
	}

	rest := raw[len(prefix):]

	switch kind {
	case ComponentNameConstructor:
		if !IsKebabCase(rest) {
			return ComponentName{}, fmt.Errorf("invalid resource name in %q: %q", raw, rest)
		}
		return ComponentName{Raw: raw, Kind: kind, Resource: rest}, nil

	case ComponentNameMethod, ComponentNameStatic:
		dot := strings.IndexByte(rest, '.')
		if dot < 0 {
			return ComponentName{}, fmt.Errorf("missing '.' in %q", raw)
		}
		resource := rest[:dot]
		method := rest[dot+1:]
		if !IsKebabCase(resource) {
			return ComponentName{}, fmt.Errorf("invalid resource name in %q: %q", raw, resource)
		}
		if !IsKebabCase(method) {
			return ComponentName{}, fmt.Errorf("invalid method name in %q: %q", raw, method)
		}
		return ComponentName{Raw: raw, Kind: kind, Resource: resource, Method: method}, nil
	}

	// unreachable
	return ComponentName{}, fmt.Errorf("unhandled bracket name: %q", raw)
}

// parseInterfaceName handles namespace:package/name[@version] names.
func parseInterfaceName(raw string) (ComponentName, error) {
	// Minimal validation: must contain ':' and '/'.
	colon := strings.IndexByte(raw, ':')
	if colon < 0 {
		return ComponentName{}, fmt.Errorf("interface name missing ':': %q", raw)
	}
	slash := strings.IndexByte(raw, '/')
	if slash < 0 || slash < colon {
		return ComponentName{}, fmt.Errorf("interface name missing '/' after ':': %q", raw)
	}
	return ComponentName{Raw: raw, Kind: ComponentNameInterface}, nil
}
