package witgen

import (
	"strings"
	"testing"
)

func TestRenderAndFormat_FileHeader(t *testing.T) {
	iface := &Interface{
		Namespace: "example",
		Package:   "demo",
		Name:      "calc",
		GoPackage: "calc",
	}
	view := ifaceView{
		Interface:  iface,
		SourceFile: "add.wit",
	}
	raw, err := renderTemplate("file_header.tmpl", view)
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	formatted, err := formatGoSource("calc.go", raw)
	if err != nil {
		t.Fatalf("formatGoSource: %v", err)
	}
	out := string(formatted)
	if !strings.Contains(out, "package calc") {
		t.Errorf("missing package declaration in:\n%s", out)
	}
	if !strings.Contains(out, "DO NOT EDIT") {
		t.Errorf("missing DO NOT EDIT marker in:\n%s", out)
	}
	if !strings.Contains(out, "from add.wit") {
		t.Errorf("missing source attribution in:\n%s", out)
	}
}

// TestEmitBindFile_BoolResults regresses a codegen bug where bool-returning
// funcs/methods/statics had their `Results` slot rendered as empty in the
// AddFunction call. PrimBool's underlying value is 0 (iota), and the
// factory.tmpl previously used {{if .Result}} on the interface field, which
// Go's text/template treats as falsy when the underlying value is 0.
func TestEmitBindFile_BoolResults(t *testing.T) {
	res := loadCached(t, "testdata/wit/bool_results.wit")
	pkg, err := LowerWorld(res, "example:demo/boolworld")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitBindFile(pkg.Interfaces[0], "bool_results.wit")
	if err != nil {
		t.Fatalf("emitBindFile: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		`b_.AddFunction("always-true"`,
		`b_.AddFunction("[method]gate.is-open"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("output missing %q. Output:\n%s", want, s)
		}
	}
	// Both bool-returning entries must declare a host.Bool result.
	if got := strings.Count(s, "Type: host.Bool"); got != 2 {
		t.Errorf("expected 2 `Type: host.Bool` result decls, got %d. Output:\n%s", got, s)
	}
	// And `close` (which has no result) must remain result-less.
	idx := strings.Index(s, `b_.AddFunction("[method]gate.close"`)
	if idx < 0 {
		t.Fatalf("missing gate.close registration. Output:\n%s", s)
	}
	closeBlock := s[idx:]
	if end := strings.Index(closeBlock, "wrap"); end > 0 {
		closeBlock = closeBlock[:end]
	}
	if strings.Contains(closeBlock, "host.Bool") {
		t.Errorf("gate.close unexpectedly declares a result. Block:\n%s", closeBlock)
	}
}

func TestEmitBindFile_Add(t *testing.T) {
	res := loadCached(t, "testdata/wit/add.wit")
	pkg, err := LowerWorld(res, "example:demo/arith")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitBindFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("emitBindFile: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"package calc",
		"type Factory struct",
		"func NewFactory(ctx context.Context, e *wacogo.Engine) (*Factory, error)",
		"func (f *Factory) NewInstance(ctx context.Context, impl Calc, deps *Deps, opts ...host.InstantiateOption)",
		"func wrapAdd(f *Factory) host.Func",
		"state_.impl.Add(",
		"uint32(stack[0])",
		"uint32(stack[1])",
		"stack[0] = uint64(result_)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q. Output:\n%s", want, s)
		}
	}
}

func TestEmitIfaceFile_Add(t *testing.T) {
	res := loadCached(t, "testdata/wit/add.wit")
	pkg, err := LowerWorld(res, "example:demo/arith")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("emitIfaceFile: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"package calc",
		"type Calc interface",
		"Add(ctx context.Context, a uint32, b uint32) (uint32, error)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q. Output:\n%s", want, s)
		}
	}
}

func TestEmitIfaceFile_EnumFlags(t *testing.T) {
	res := loadCached(t, "testdata/wit/enum_flags.wit")
	pkg, err := LowerWorld(res, "example:demo/efhost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("emitIfaceFile: %v", err)
	}
	s := string(out)
	// Normalize whitespace so gofmt column-alignment doesn't break checks.
	normalized := normalizeSpaces(s)
	for _, want := range []string{
		"type Direction uint8",
		"DirectionNorth Direction = 0",
		"DirectionSouth Direction = 1",
		"case DirectionNorth:",
		"return \"north\"",
		"type Perms uint32",
		"PermsRead Perms = 1 << 0",
		"PermsWrite Perms = 1 << 1",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("missing %q. Output:\n%s", want, s)
		}
	}
}

// normalizeSpaces collapses runs of spaces/tabs into a single space per line.
// Used to keep substring assertions robust against gofmt column alignment.
func normalizeSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		// Preserve leading whitespace structure but collapse internal runs.
		leadingEnd := 0
		for leadingEnd < len(l) && (l[leadingEnd] == ' ' || l[leadingEnd] == '\t') {
			leadingEnd++
		}
		leading := l[:leadingEnd]
		rest := l[leadingEnd:]
		// Collapse runs of spaces/tabs in rest.
		var b strings.Builder
		prevSpace := false
		for _, r := range rest {
			if r == ' ' || r == '\t' {
				if !prevSpace {
					b.WriteByte(' ')
				}
				prevSpace = true
			} else {
				b.WriteRune(r)
				prevSpace = false
			}
		}
		lines[i] = leading + b.String()
	}
	return strings.Join(lines, "\n")
}

func TestEmitIfaceFile_OptionResult(t *testing.T) {
	res := loadCached(t, "testdata/wit/option_result.wit")
	pkg, err := LowerWorld(res, "example:demo/tryhost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("emitIfaceFile: %v", err)
	}
	s := string(out)
	normalized := normalizeSpaces(s)
	for _, want := range []string{
		"type OptionString struct",
		"IsSome bool",
		"Value string",
		"func SomeString(v string) OptionString",
		"func NoneString() OptionString",
		// For top-level result<u32, string>, the interface method returns
		// (ResultU32String, error) — the inner sealed Result carries the
		// business outcome and the outer error covers trap/infrastructure.
		"Parse(ctx context.Context, s string) (ResultU32String, error)",
		"Find(ctx context.Context, id uint32) (OptionString, error)",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("missing %q. Output:\n%s", want, s)
		}
	}
}

func TestEmitIfaceFile_Record(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_record_only.wit")
	pkg, err := LowerWorld(res, "example:demo/pointshost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("%v", err)
	}
	s := string(out)
	for _, want := range []string{
		"type Points interface",
		"Midpoint(ctx context.Context, a Point, b Point) (Point, error)",
		"type Point struct",
		"X uint32",
		"Y uint32",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q. Output:\n%s", want, s)
		}
	}
}

func TestEmitIfaceFile_Variant(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_variant_only.wit")
	pkg, err := LowerWorld(res, "example:demo/shapeshost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("%v", err)
	}
	s := string(out)
	for _, want := range []string{
		"type ShapeKind interface{ isShapeKind() }",
		"type ShapeKindCircle struct",
		"Value uint32",
		"type ShapeKindRect struct",
		"Value Point",
		"type ShapeKindNone struct",
		"func (ShapeKindCircle) isShapeKind() {}",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q. Output:\n%s", want, s)
		}
	}
}

func TestEmitIfaceFile_Resource(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_counter.wit")
	pkg, err := LowerWorld(res, "example:demo/countershost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("%v", err)
	}
	s := string(out)
	for _, want := range []string{
		"type Counter interface",
		"Increment(ctx context.Context) error",
		"Current(ctx context.Context) (uint32, error)",
		"type Counters interface",
		"NewCounter(ctx context.Context, initial uint32) (*CounterHandle, error)",
		"type CounterHandle struct",
		"func NewCounterHandle(impl Counter) *CounterHandle",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q. Output:\n%s", want, s)
		}
	}
}

func TestEmitIfaceFile_TupleDeclarations(t *testing.T) {
	res := loadCached(t, "testdata/wit/types_compound.wit")
	pkg, err := LowerWorld(res, "example:demo/combo")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	out, err := emitIfaceFile(pkg.Interfaces[0], "add.wit")
	if err != nil {
		t.Fatalf("emitIfaceFile: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"type Mix interface",
		"Greet(ctx context.Context, name string) (string, error)",
		"Sum(ctx context.Context, xs []uint32) (uint32, error)",
		"Pair(ctx context.Context) (TupleU32U32, error)",
		"Nested(ctx context.Context) ([]TupleStringU32, error)",
		"type TupleU32U32 struct",
		"F0 uint32",
		"F1 uint32",
		"type TupleStringU32 struct",
		"F0 string",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q. Output:\n%s", want, s)
		}
	}
}
