package witgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"text/template"

	"golang.org/x/tools/go/ast/astutil"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

var tmplSet = template.Must(
	template.New("witgen").
		Funcs(template.FuncMap{
			"goType":                    GoTypeOf,
			"tupleName":                 func(t *TypeTuple) string { return TypeName(t) },
			"optionName":                func(t *TypeOption) string { return TypeName(t) },
			"resultDecl":                resultDecl,
			"elemName":                  TypeName,
			"funcSigForSide":            funcSigForSide,
			"methodSigForSide":          methodSigForSide,
			"ctorSigForSide":            ctorSigForSide,
			"staticSigForSide":          staticSigForSide,
			"resourceFieldName":         resourceFieldName,
			"importedNominalAddTypeStmt": importedNominalAddTypeStmt,
			"directNominals":             directNominals,
			"lcfirst":                   lcfirst,
			"inc":                       func(i int) int { return i + 1 },
			"goDoc":                     goDocComment,
		}).
		ParseFS(templatesFS, "templates/*.tmpl"),
)

// resultDecl emits the Go declarations for a sealed Result type:
// a marker interface plus Ok/Err case structs (each with a Value
// field when the corresponding arm carries a payload). Used by
// iface.tmpl to render every *TypeResult in the IR.
func resultDecl(r *TypeResult) string {
	name := TypeName(r)
	var b strings.Builder
	fmt.Fprintf(&b, "// %s corresponds to a WIT result<...> type.\n", name)
	fmt.Fprintf(&b, "type %s interface { is%s() }\n", name, name)
	if r.OK == nil {
		fmt.Fprintf(&b, "type %sOk struct{}\n", name)
	} else {
		fmt.Fprintf(&b, "type %sOk struct { Value %s }\n", name, GoTypeOf(r.OK))
	}
	if r.Err == nil {
		fmt.Fprintf(&b, "type %sErr struct{}\n", name)
	} else {
		fmt.Fprintf(&b, "type %sErr struct { Value %s }\n", name, GoTypeOf(r.Err))
	}
	fmt.Fprintf(&b, "func (%sOk) is%s() {}\n", name, name)
	fmt.Fprintf(&b, "func (%sErr) is%s() {}\n", name, name)
	return b.String()
}

// directNominals returns the subset of ns that are not Transitive — i.e.
// nominals directly referenced in the importing iface's own signatures.
// Used by factory.tmpl to skip AddType emission for transitively-pulled
// imports (which exist only to drive qualifier rewriting on imported
// resource methods, not for host.Builder type registration).
func directNominals(ns []ImportedNominal) []ImportedNominal {
	var out []ImportedNominal
	for _, n := range ns {
		if !n.Transitive {
			out = append(out, n)
		}
	}
	return out
}

// lcfirst lowercases the first rune of s. Returns s unchanged if empty.
func lcfirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// funcSigForSide is like funcSigExpr but uses goTypeForSide for resource-typed
// params/results, and emits the (T, error) / error return shape used by the
// host-impl and caller-wrapper sides. useImpl=true renders owned-resource
// types as <R>Impl; useImpl=false renders them as <R>.
func funcSigForSide(useImpl bool, fn *Func) string {
	ps := []string{"ctx context.Context"}
	for _, p := range fn.Params {
		ps = append(ps, p.GoName+" "+goTypeForSide(p.Type, useImpl))
	}
	paramList := strings.Join(ps, ", ")
	return fmt.Sprintf("%s(%s) %s", fn.GoName, paramList, returnTypeForSide(fn.Result, useImpl))
}

// ctorSigForSide returns the constructor signature for a resource's hoisted
// constructor as it appears on the parent interface. After the handle-
// management refactor the return type is uniformly (*<R>Handle, error). The
// useImpl parameter is retained for callsite compatibility but has no
// effect on resource rendering.
func ctorSigForSide(useImpl bool, rt *TypeResource) string {
	ps := []string{"ctx context.Context"}
	for _, p := range rt.Ctor.Params {
		ps = append(ps, p.GoName+" "+GoTypeOf(p.Type))
	}
	return fmt.Sprintf("New%s(%s) (*%sHandle, error)", rt.WrapName, strings.Join(ps, ", "), rt.WrapName)
}

// staticSigForSide returns the static-function signature for a resource as it
// appears on the parent interface. useImpl selects the resource return type.
func staticSigForSide(useImpl bool, rt *TypeResource, s ResourceStatic) string {
	ps := []string{"ctx context.Context"}
	for _, p := range s.Params {
		ps = append(ps, p.GoName+" "+goTypeForSide(p.Type, useImpl))
	}
	return fmt.Sprintf("%s(%s) %s", s.GoName, strings.Join(ps, ", "), returnTypeForSide(s.Result, useImpl))
}

// methodSigForSide is like methodSigExpr but uses goTypeForSide for
// resource-typed params/results, and emits the (T, error) / error return
// shape used by both sides.
func methodSigForSide(useImpl bool, m ResourceMethod) string {
	ps := []string{"ctx context.Context"}
	for _, p := range m.Params {
		ps = append(ps, p.GoName+" "+goTypeForSide(p.Type, useImpl))
	}
	return fmt.Sprintf("%s(%s) %s", m.GoName, strings.Join(ps, ", "), returnTypeForSide(m.Result, useImpl))
}

// returnTypeForSide returns the Go return-type expression for a user-facing
// method's result. Unlike resultFuncSigReturn, *TypeResult is rendered as
// the sealed interface type (the inner business outcome) and the outer
// error captures trap/infrastructure failures:
//
//	nil       → error
//	T         → (T, error)
//	*TypeResult → (<SealedName>, error)
func returnTypeForSide(t Type, useImpl bool) string {
	if t == nil {
		return "error"
	}
	return fmt.Sprintf("(%s, error)", goTypeForSide(t, useImpl))
}

// renderTemplate executes the named template against data into a
// bytes.Buffer. Returns an error if the template name doesn't exist
// or execution fails.
func renderTemplate(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	t := tmplSet.Lookup(name)
	if t == nil {
		return nil, fmt.Errorf("witgen: template %q not found", name)
	}
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("witgen: render %q: %w", name, err)
	}
	return buf.Bytes(), nil
}

// formatGoSource parses src, drops any unused imports, and gofmts the
// result. Generated templates emit a superset of the imports each
// variant might need; the AST-level prune keeps fixtures compilable
// without the full goimports module-resolver walk (which dominated
// witgen test time).
func formatGoSource(filename string, src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("witgen: parse %s: %w\n--- unformatted ---\n%s", filename, err, src)
	}
	// Snapshot first — DeleteImport mutates f.Imports.
	imps := make([]struct {
		path, name string
	}, 0, len(f.Imports))
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imps = append(imps, struct {
			path, name string
		}{path, name})
	}
	for _, imp := range imps {
		if astutil.UsesImport(f, imp.path) {
			continue
		}
		if imp.name != "" {
			astutil.DeleteNamedImport(fset, f, imp.name, imp.path)
		} else {
			astutil.DeleteImport(fset, f, imp.path)
		}
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, fmt.Errorf("witgen: format %s: %w\n--- unformatted ---\n%s", filename, err, src)
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("witgen: gofmt %s: %w\n--- pre-gofmt ---\n%s", filename, err, buf.Bytes())
	}
	return out, nil
}
