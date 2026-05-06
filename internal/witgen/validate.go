package witgen

import (
	"errors"
	"fmt"
)

// ValidationError describes a problem with the lowered IR. Returned by
// Validate; aggregated errors are wrapped via errors.Join.
type ValidationError struct {
	Interface *Interface  // may be nil for cross-interface issues
	Function  string      // empty if not function-scoped
	Message   string
}

func (e *ValidationError) Error() string {
	var prefix string
	if e.Interface != nil {
		prefix = e.Interface.Namespace + ":" + e.Interface.Package + "/" + e.Interface.Name
		if e.Function != "" {
			prefix += "." + e.Function
		}
		prefix += ": "
	}
	return prefix + e.Message
}

// Validate runs pre-emit checks on the lowered Package and returns
// errors for any issues that would produce broken or surprising output.
// Returns nil if everything is OK.
func Validate(pkg *Package) error {
	var errs []error
	for _, iface := range pkg.Interfaces {
		if err := validateInterface(iface); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}

func validateInterface(iface *Interface) error {
	var errs []error

	// Check 1: Go-name collisions across freestanding funcs and resource methods/ctors/statics.
	// Two distinct WIT identifiers may map to the same Go identifier (e.g., "get_value" and "get-value").
	seen := map[string]string{} // GoName → first WIT name
	for _, fn := range iface.Funcs {
		if prev, ok := seen[fn.GoName]; ok && prev != fn.WitName {
			errs = append(errs, &ValidationError{
				Interface: iface,
				Function:  fn.WitName,
				Message:   fmt.Sprintf("Go-name collision: %q maps to %q which is also used by %q", fn.WitName, fn.GoName, prev),
			})
		}
		seen[fn.GoName] = fn.WitName
	}
	for _, rt := range iface.Resources {
		if rt.Ctor != nil {
			ctorGoName := "New" + rt.GoName
			if prev, ok := seen[ctorGoName]; ok {
				errs = append(errs, &ValidationError{
					Interface: iface,
					Function:  "[constructor]" + rt.Name,
					Message:   fmt.Sprintf("Go-name collision: ctor %q (Go %q) collides with %q", rt.Name, ctorGoName, prev),
				})
			}
			seen[ctorGoName] = "[constructor]" + rt.Name
		}
		for _, s := range rt.Statics {
			if prev, ok := seen[s.GoName]; ok {
				errs = append(errs, &ValidationError{
					Interface: iface,
					Function:  "[static]" + rt.Name + "." + s.Name,
					Message:   fmt.Sprintf("Go-name collision: static %q (Go %q) collides with %q", s.Name, s.GoName, prev),
				})
			}
			seen[s.GoName] = "[static]" + rt.Name + "." + s.Name
		}
	}

	// Check 2: Reserved Go keywords used as parameter or method names.
	// (We don't catch all collisions but catch obvious cases.)
	for _, fn := range iface.Funcs {
		for _, p := range fn.Params {
			if isGoReserved(p.GoName) {
				errs = append(errs, &ValidationError{
					Interface: iface,
					Function:  fn.WitName,
					Message:   fmt.Sprintf("parameter name %q is a Go reserved word; rename in WIT", p.GoName),
				})
			}
		}
	}

	// Check 3 (removed): the old guard rejected non-string err arms but the
	// emit code fully handles enum/variant/record/flags error-arm payloads.

	// Check 4: a WIT-level nominal name appearing twice across own-iface
	// declarations and imported declarations would produce two
	// `b.AddType("foo", ...)` calls in the importer's NewFactory, which
	// the host builder rejects. Surface a clear error.
	witNameSeen := map[string]string{} // WIT name → "<kind> from <iface>"
	ownLabel := func(kind string) string { return fmt.Sprintf("%s from %q (own)", kind, iface.Name) }
	for _, r := range iface.Records {
		witNameSeen[r.Name] = ownLabel("record")
	}
	for _, v := range iface.Variants {
		witNameSeen[v.Name] = ownLabel("variant")
	}
	for _, e := range iface.Enums {
		witNameSeen[e.Name] = ownLabel("enum")
	}
	for _, f := range iface.Flags {
		witNameSeen[f.Name] = ownLabel("flags")
	}
	for _, imp := range iface.Imports {
		check := func(kind string, ns []ImportedNominal) {
			for _, n := range ns {
				if prev, dup := witNameSeen[n.Name]; dup {
					errs = append(errs, &ValidationError{
						Interface: iface,
						Message: fmt.Sprintf("imported nominal %q (%s from %q) collides with %s", n.Name, kind, imp.Name, prev),
					})
				}
				witNameSeen[n.Name] = fmt.Sprintf("%s from %q", kind, imp.Name)
			}
		}
		check("record", imp.Records)
		check("variant", imp.Variants)
		check("enum", imp.Enums)
		check("flags", imp.Flags)
	}

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}

// isGoReserved returns true for Go's reserved identifiers (keywords +
// predeclared types/funcs/constants that would shadow if used as locals).
func isGoReserved(name string) bool {
	switch name {
	case "break", "case", "chan", "const", "continue", "default", "defer",
		"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
		"interface", "map", "package", "range", "return", "select", "struct",
		"switch", "type", "var":
		return true
	case "true", "false", "nil", "iota":
		return true
	case "any", "bool", "byte", "comparable", "complex64", "complex128",
		"error", "float32", "float64", "int", "int8", "int16", "int32", "int64",
		"rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	}
	return false
}
