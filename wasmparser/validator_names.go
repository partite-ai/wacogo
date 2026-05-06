package wasmparser

import (
	"fmt"
	"strconv"
	"strings"
)

// ErrNotValidExternName is returned when a name is not valid as any extern name
// (e.g., an import-only name format used as an export).
type ErrNotValidExternName struct {
	msg string
}

func (e *ErrNotValidExternName) Error() string { return e.msg }

// ValidateExternName validates a component extern name (used for both imports and exports).
// kind should be "import" or "export".
func ValidateExternName(raw string, kind string) error {
	if raw == "" {
		return fmt.Errorf("`` is not in kebab case")
	}

	// Check for bracket-prefixed names
	if strings.HasPrefix(raw, "[constructor]") {
		rest := raw[len("[constructor]"):]
		if !IsKebabCase(rest) {
			return fmt.Errorf("`%s` is not in kebab case", rest)
		}
		return nil
	}
	if strings.HasPrefix(raw, "[method]") {
		rest := raw[len("[method]"):]
		resource, method, ok := strings.Cut(rest, ".")
		if !ok {
			return fmt.Errorf("failed to find `.` character")
		}
		if !IsKebabCase(resource) {
			return fmt.Errorf("`%s` is not in kebab case", resource)
		}
		if !IsKebabCase(method) {
			return fmt.Errorf("`%s` is not in kebab case", method)
		}
		return nil
	}
	if strings.HasPrefix(raw, "[static]") {
		rest := raw[len("[static]"):]
		resource, method, ok := strings.Cut(rest, ".")
		if !ok {
			return fmt.Errorf("failed to find `.` character")
		}
		if !IsKebabCase(resource) {
			return fmt.Errorf("`%s` is not in kebab case", resource)
		}
		if !IsKebabCase(method) {
			return fmt.Errorf("`%s` is not in kebab case", method)
		}
		return nil
	}

	// Dependency names (only valid for imports)
	if strings.HasPrefix(raw, "unlocked-dep=") {
		if kind == "export" {
			return fmt.Errorf("`%s` is not in kebab case", raw)
		}
		return validateUnlockedDep(raw[len("unlocked-dep="):])
	}
	if strings.HasPrefix(raw, "locked-dep=") {
		if kind == "export" {
			return fmt.Errorf("`%s` is not in kebab case", raw)
		}
		return validateLockedDep(raw[len("locked-dep="):])
	}

	// URL names (only valid for imports)
	if strings.HasPrefix(raw, "url=") {
		if kind == "export" {
			return fmt.Errorf("`%s` is not in kebab case", raw)
		}
		return validateURL(raw[len("url="):])
	}

	// Hash/integrity names (only valid for imports)
	if strings.HasPrefix(raw, "integrity=") {
		if kind == "export" {
			return fmt.Errorf("`%s` is not in kebab case", raw)
		}
		return validateIntegrity(raw[len("integrity="):])
	}

	// Check for bracket prefix that doesn't match known patterns
	if strings.HasPrefix(raw, "[") {
		return fmt.Errorf("not a valid %s name: unknown bracket prefix in `%s`", kind, raw)
	}

	// Names containing '=' that didn't match any known prefix are not valid extern names.
	if strings.ContainsRune(raw, '=') {
		return &ErrNotValidExternName{msg: fmt.Sprintf("`%s` is not in kebab case", raw)}
	}

	// Interface name: contains ':'
	if strings.ContainsRune(raw, ':') {
		return validateInterfaceName(raw)
	}

	// Plain kebab-case label
	if !IsKebabCase(raw) {
		return fmt.Errorf("`%s` is not in kebab case", raw)
	}
	return nil
}

func validateInterfaceName(raw string) error {
	// Format: namespace:package/name@version
	// Where namespace and package segments are lowercase-kebab-case
	rest := raw

	// Parse namespace (before first ':')
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return fmt.Errorf("expected `:` in interface name `%s`", raw)
	}
	namespace := rest[:colon]
	rest = rest[colon+1:]

	if err := validateNamespaceOrPackage(namespace); err != nil {
		return err
	}

	// Parse package name (before '/')
	// First check for another colon (invalid - only one colon allowed)
	colonIdx := strings.IndexByte(rest, ':')
	slash := strings.IndexByte(rest, '/')
	if colonIdx >= 0 && (slash < 0 || colonIdx < slash) {
		// There's a colon before the slash (or no slash) - validate the part before the colon
		// as the package name, then report expected '/'
		pkg := rest[:colonIdx]
		if err := validateNamespaceOrPackage(pkg); err != nil {
			return err
		}
		return fmt.Errorf("expected `/` after package name")
	}
	if slash < 0 {
		// No slash found
		if rest == "" {
			return fmt.Errorf("`` is not in kebab case")
		}
		return fmt.Errorf("expected `/` after package name")
	}
	pkg := rest[:slash]
	rest = rest[slash+1:]

	if err := validateNamespaceOrPackage(pkg); err != nil {
		return err
	}

	// Check for trailing slashes (e.g. "foo:bar/baz/qux")
	if slashIdx := strings.IndexByte(rest, '/'); slashIdx >= 0 {
		// Parse the interface name up to the trailing slash
		name := rest[:slashIdx]
		trailing := rest[slashIdx:]
		if err := validateInterfaceLabel(name); err != nil {
			return err
		}
		return fmt.Errorf("trailing characters found: `%s`", trailing)
	}

	// Parse interface name (before '@' or end)
	at := strings.IndexByte(rest, '@')
	var name string
	if at >= 0 {
		name = rest[:at]
		rest = rest[at+1:]
	} else {
		name = rest
		rest = ""
	}

	if err := validateInterfaceLabel(name); err != nil {
		return err
	}

	// If there's a version, validate it
	if at >= 0 {
		if rest == "" {
			return fmt.Errorf("empty string")
		}
		if err := validateSemverStrict(rest); err != nil {
			return err
		}
	}

	return nil
}

// validateNamespaceOrPackage validates a namespace or package name.
func validateNamespaceOrPackage(s string) error {
	if s == "" {
		return fmt.Errorf("`` is not in kebab case")
	}
	// If it passes mixed-case kebab validation but not lowercase kebab,
	// it means the name has valid structure but uses uppercase characters.
	if IsKebabCase(s) && !isLowercaseKebab(s) {
		// Find the first uppercase character
		for _, c := range []byte(s) {
			if c >= 'A' && c <= 'Z' {
				return fmt.Errorf("character `%c` is not lowercase in package name/namespace", c)
			}
		}
	}
	if !isLowercaseKebab(s) {
		return fmt.Errorf("`%s` is not in kebab case", s)
	}
	return nil
}

// validateKebabName validates that a name segment is valid lowercase kebab-case.
// Returns an error matching the Rust wasmparser's format.
func validateKebabName(s string) error {
	if s == "" {
		return fmt.Errorf("`` is not in kebab case")
	}
	if !isLowercaseKebab(s) {
		return fmt.Errorf("`%s` is not in kebab case", s)
	}
	return nil
}

// validateInterfaceLabel validates the interface-name segment of an interface
// extern name (the portion after the slash). Interface labels permit
// mixed-case kebab per the Component Model spec.
func validateInterfaceLabel(s string) error {
	if s == "" {
		return fmt.Errorf("`` is not in kebab case")
	}
	if !IsKebabCase(s) {
		return fmt.Errorf("`%s` is not in kebab case", s)
	}
	return nil
}

// validateSemverStrict validates a semver string with detailed error messages
// matching the Rust wasmparser's format.
func validateSemverStrict(s string) error {
	if s == "" {
		return fmt.Errorf("empty string")
	}

	pos := 0
	// Parse major
	major, newPos, err := parseSemverNumber(s, pos)
	if err != nil {
		return err
	}
	_ = major
	pos = newPos

	if pos >= len(s) {
		return fmt.Errorf("unexpected end of input")
	}
	if s[pos] != '.' {
		return fmt.Errorf("unexpected character '%c'", s[pos])
	}
	pos++ // consume '.'

	// Parse minor
	minor, newPos, err := parseSemverNumber(s, pos)
	if err != nil {
		return err
	}
	_ = minor
	pos = newPos

	if pos >= len(s) {
		return fmt.Errorf("unexpected end of input")
	}
	if s[pos] != '.' {
		return fmt.Errorf("unexpected character '%c'", s[pos])
	}
	pos++ // consume '.'

	// Parse patch
	patch, newPos, err := parseSemverNumber(s, pos)
	if err != nil {
		return err
	}
	_ = patch
	pos = newPos

	// Parse optional pre-release (-...)
	if pos < len(s) && s[pos] == '-' {
		pos++ // consume '-'
		newPos, err := parseSemverIdentifiers(s, pos, true)
		if err != nil {
			return err
		}
		pos = newPos
	}

	// Parse optional build metadata (+...)
	if pos < len(s) && s[pos] == '+' {
		pos++ // consume '+'
		newPos, err := parseSemverIdentifiers(s, pos, false)
		if err != nil {
			return err
		}
		pos = newPos
	}

	if pos < len(s) {
		return fmt.Errorf("unexpected character '%c'", s[pos])
	}

	return nil
}

func parseSemverNumber(s string, pos int) (uint64, int, error) {
	if pos >= len(s) {
		return 0, pos, fmt.Errorf("unexpected end of input")
	}
	c := s[pos]
	if c < '0' || c > '9' {
		return 0, pos, fmt.Errorf("unexpected character '%c'", c)
	}
	start := pos
	for pos < len(s) && s[pos] >= '0' && s[pos] <= '9' {
		pos++
	}
	val, err := strconv.ParseUint(s[start:pos], 10, 64)
	if err != nil {
		return 0, pos, fmt.Errorf("number too large")
	}
	return val, pos, nil
}

func parseSemverIdentifiers(s string, pos int, stopAtPlus bool) (int, error) {
	// Parse dot-separated identifiers (pre-release or build metadata)
	// Identifiers can contain alphanumeric characters and hyphens.
	// Pre-release identifiers stop at '+', build metadata stops at end of string.
	start := pos
	for pos < len(s) {
		if stopAtPlus && s[pos] == '+' {
			break
		}
		if s[pos] == '.' {
			seg := s[start:pos]
			if seg == "" {
				return pos, fmt.Errorf("empty identifier segment")
			}
			pos++
			start = pos
			continue
		}
		if !isAlphanumericOrHyphen(s[pos]) {
			return pos, fmt.Errorf("unexpected character '%c'", s[pos])
		}
		pos++
	}
	seg := s[start:pos]
	if seg == "" {
		return pos, fmt.Errorf("empty identifier segment")
	}
	return pos, nil
}

func isAlphanumericOrHyphen(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
}

func isLowercaseKebab(s string) bool {
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
	for _, c := range []byte(s) {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func validateSemver(s string) error {
	return validateSemverStrict(s)
}

func validateUnlockedDep(rest string) error {
	// unlocked-dep=<pkgquery>
	if !strings.HasPrefix(rest, "<") {
		return fmt.Errorf("expected `<` at `%s`", rest)
	}
	rest = rest[1:]
	// Parse the query, leaving the '>' for us to consume
	remaining, err := validatePkgNameQueryRemaining(rest)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(remaining, ">") {
		return fmt.Errorf("expected `>` at `%s`", remaining)
	}
	remaining = remaining[1:]
	if remaining != "" {
		return fmt.Errorf("trailing characters found: `%s`", remaining)
	}
	return nil
}

func validateLockedDep(rest string) error {
	// locked-dep=<pkgname>,integrity=<hash> or locked-dep=<pkgname>
	if !strings.HasPrefix(rest, "<") {
		return fmt.Errorf("expected `<` at `%s`", rest)
	}
	rest = rest[1:]
	// Find first > to close the pkg name
	gt := strings.IndexByte(rest, '>')
	if gt < 0 {
		// No closing >: validate the content first, then report missing >
		if err := validatePkgPath(rest); err != nil {
			return err
		}
		return fmt.Errorf("expected `>` at ``")
	}
	inner := rest[:gt]
	rest = rest[gt+1:]

	if err := validatePkgPath(inner); err != nil {
		return err
	}

	// Check for trailing content
	if rest == "" {
		return nil
	}

	// Expect ,integrity=<hash>
	if !strings.HasPrefix(rest, ",integrity=<") {
		if strings.HasPrefix(rest, ",") {
			return fmt.Errorf("expected `integrity=<` after `,`")
		}
		return fmt.Errorf("trailing characters found: `%s`", rest)
	}
	hashRest := rest[len(",integrity=<"):]
	gt2 := strings.IndexByte(hashRest, '>')
	if gt2 < 0 {
		return fmt.Errorf("expected `>` after hash")
	}
	if gt2 != len(hashRest)-1 {
		return fmt.Errorf("trailing characters found: `%s`", hashRest[gt2+1:])
	}
	return validateHashValue(hashRest[:gt2])
}

// validatePkgNameQueryRemaining parses a pkg name query and returns the remaining
// unparsed string. The query is namespace:name[/interface][@version|@*|@{range}].
// The caller should check that the remaining string is empty or as expected.
func validatePkgNameQueryRemaining(s string) (string, error) {
	// pkg name query is namespace:name[/interface][@version|@*|@{range}]
	namespace, rest, hasColon := strings.Cut(s, ":")
	if !hasColon {
		// No colon found. Parse name up to '>' boundary or end.
		name, gtRest, hasGT := strings.Cut(s, ">")
		if err := validateKebabName(name); err != nil {
			return "", err
		}
		if hasGT {
			return ">" + gtRest, nil
		}
		return "", nil
	}

	if err := validateKebabName(namespace); err != nil {
		return "", err
	}

	// Parse package name (up to '/', '@', '{', '>', or end)
	pkgEnd := len(rest)
	for i, c := range rest {
		if c == '/' || c == '@' || c == '{' || c == '>' {
			pkgEnd = i
			break
		}
	}
	pkg := rest[:pkgEnd]
	rest = rest[pkgEnd:]

	if err := validateKebabName(pkg); err != nil {
		return "", err
	}

	// Optional interface projection
	if len(rest) > 0 && rest[0] == '/' {
		rest = rest[1:]
		// Parse interface name (up to '@' or '{' or '>' or end)
		nameEnd := len(rest)
		for i, c := range rest {
			if c == '@' || c == '{' || c == '>' {
				nameEnd = i
				break
			}
		}
		name := rest[:nameEnd]
		rest = rest[nameEnd:]
		if err := validateKebabName(name); err != nil {
			return "", err
		}
	}

	// Optional version (for pkg name queries, must be * or {range})
	if len(rest) > 0 && rest[0] == '@' {
		rest = rest[1:]
		if len(rest) > 0 && rest[0] == '*' {
			rest = rest[1:]
			return rest, nil
		}
		if len(rest) > 0 && rest[0] == '{' {
			remaining, err := validateSemverRangeRemaining(rest)
			if err != nil {
				return "", err
			}
			return remaining, nil
		}
		// For queries, @ must be followed by * or {range}
		return "", fmt.Errorf("expected `{` at `%s`", rest)
	}

	return rest, nil
}

func validatePkgPath(s string) error {
	// pkg path is namespace:name[/interface][@version]
	namespace, rest, hasColon := strings.Cut(s, ":")
	if !hasColon {
		return validateKebabName(s)
	}

	if err := validateKebabName(namespace); err != nil {
		return err
	}

	// Parse package name (up to '/', '@', or end)
	pkgEnd := len(rest)
	for i, c := range rest {
		if c == '/' || c == '@' {
			pkgEnd = i
			break
		}
	}
	pkg := rest[:pkgEnd]
	rest = rest[pkgEnd:]

	if err := validateKebabName(pkg); err != nil {
		return err
	}

	// Optional interface projection
	if len(rest) > 0 && rest[0] == '/' {
		rest = rest[1:]
		nameEnd := len(rest)
		for i, c := range rest {
			if c == '@' {
				nameEnd = i
				break
			}
		}
		name := rest[:nameEnd]
		rest = rest[nameEnd:]
		if err := validateKebabName(name); err != nil {
			return err
		}
	}

	// Optional version
	if len(rest) > 0 && rest[0] == '@' {
		rest = rest[1:]
		if rest == "" {
			return fmt.Errorf("`` is not a valid semver")
		}
		return validateSemver(rest)
	}

	return nil
}

// validateSemverRangeRemaining parses a {>=X <Y} range and returns remaining unparsed text.
func validateSemverRangeRemaining(s string) (string, error) {
	if !strings.HasPrefix(s, "{") {
		return "", fmt.Errorf("expected `{` at `%s`", s)
	}
	// Find the closing }
	closeBrace := strings.IndexByte(s, '}')
	if closeBrace < 0 {
		return "", fmt.Errorf("expected `}` at end of range")
	}
	inner := s[1:closeBrace]
	remaining := s[closeBrace+1:]

	if inner == "" {
		return "", fmt.Errorf("expected `>=` or `<` at start of version range")
	}

	pos := 0
	for pos < len(inner) {
		// Skip spaces
		for pos < len(inner) && inner[pos] == ' ' {
			pos++
		}
		if pos >= len(inner) {
			break
		}

		if strings.HasPrefix(inner[pos:], ">=") {
			pos += 2
			// Find end of version (next space or end)
			end := strings.IndexByte(inner[pos:], ' ')
			var ver string
			if end >= 0 {
				ver = inner[pos : pos+end]
				pos = pos + end
			} else {
				ver = inner[pos:]
				pos = len(inner)
			}
			if err := validateSemver(ver); err != nil {
				return "", fmt.Errorf("`%s` is not a valid semver", ver)
			}
		} else if strings.HasPrefix(inner[pos:], "<") {
			pos += 1
			// Take everything remaining as the version
			ver := inner[pos:]
			pos = len(inner)
			if err := validateSemver(ver); err != nil {
				return "", fmt.Errorf("`%s` is not a valid semver", ver)
			}
		} else {
			return "", fmt.Errorf("expected `>=` or `<` at start of version range")
		}
	}
	return remaining, nil
}

func validateURL(rest string) error {
	if !strings.HasPrefix(rest, "<") {
		return fmt.Errorf("expected `<` at `%s`", rest)
	}
	rest = rest[1:]

	gt := strings.IndexByte(rest, '>')
	if gt < 0 {
		return fmt.Errorf("failed to find `>`")
	}
	url := rest[:gt]
	rest = rest[gt+1:]

	// URL cannot contain '<'
	if strings.ContainsRune(url, '<') {
		return fmt.Errorf("url cannot contain `<`")
	}

	// Optional ,integrity=<hash> suffix
	if rest != "" {
		if !strings.HasPrefix(rest, ",integrity=<") {
			return fmt.Errorf("expected `integrity=<` after URL")
		}
		hashRest := rest[len(",integrity=<"):]
		gt2 := strings.IndexByte(hashRest, '>')
		if gt2 < 0 {
			return fmt.Errorf("expected `>` after hash")
		}
		if gt2 != len(hashRest)-1 {
			return fmt.Errorf("trailing characters found: `%s`", hashRest[gt2+1:])
		}
		return validateHashValue(hashRest[:gt2])
	}
	return nil
}

func validateIntegrity(rest string) error {
	if !strings.HasPrefix(rest, "<") {
		return fmt.Errorf("expected `<` at `%s`", rest)
	}
	rest = rest[1:]
	gt := strings.IndexByte(rest, '>')
	if gt < 0 {
		return fmt.Errorf("expected `>` after integrity value")
	}
	if gt != len(rest)-1 {
		return fmt.Errorf("trailing characters found: `%s`", rest[gt+1:])
	}
	return validateHashValue(rest[:gt])
}

func validateHashValue(s string) error {
	if s == "" {
		return fmt.Errorf("integrity hash cannot be empty")
	}
	// Multiple space-separated hashes are allowed (SRI format)
	hashes := strings.Fields(s)
	if len(hashes) == 0 {
		return fmt.Errorf("integrity hash cannot be empty")
	}
	for _, hash := range hashes {
		if err := validateSingleHash(hash); err != nil {
			return err
		}
	}
	return nil
}

func validateSingleHash(s string) error {
	algo, rest, ok := strings.Cut(s, "-")
	if !ok {
		return fmt.Errorf("expected `-` after hash algorithm")
	}

	switch algo {
	case "sha256", "sha384", "sha512":
		// ok
	default:
		return fmt.Errorf("unrecognized hash algorithm: `%s`", algo)
	}

	// Handle optional ?options suffix
	b64, _, _ := strings.Cut(rest, "?")

	if b64 == "" {
		return fmt.Errorf("not valid base64: ``")
	}

	if !isBase64(b64) {
		return fmt.Errorf("not valid base64: `%s`", b64)
	}
	return nil
}

func isBase64(s string) bool {
	if s == "" {
		return false
	}
	equals := 0
	for i, c := range []byte(s) {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '+' || c == '/':
			if equals > 0 {
				// Data character after padding
				return false
			}
		case c == '=':
			if i == 0 || equals >= 2 {
				return false
			}
			equals++
		default:
			return false
		}
	}
	return true
}

// ValidateImportName validates a component import name.
func ValidateImportName(name ComponentImportName) error {
	switch name.Kind {
	case ImportNamePlain:
		return ValidateExternName(name.Name, "import")
	case ImportNameScoped:
		return ValidateExternName(name.Name, "import")
	case ImportNameVersioned:
		return ValidateExternName(name.Name, "import")
	}
	return nil
}

// ValidateExportNameStr validates a component export name.
func ValidateExportNameStr(name ComponentExportName) error {
	switch name.Kind {
	case ImportNamePlain:
		return ValidateExternName(name.Name, "export")
	case ImportNameScoped:
		return ValidateExternName(name.Name, "export")
	case ImportNameVersioned:
		return ValidateExternName(name.Name, "export")
	}
	return nil
}
