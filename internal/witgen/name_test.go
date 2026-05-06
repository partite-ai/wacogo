package witgen

import "testing"

func TestGoName_Exported(t *testing.T) {
	cases := []struct{ in, want string }{
		{"add", "Add"},
		{"get-value", "GetValue"},
		{"http-status", "HTTPStatus"},
		{"json-parser", "JSONParser"},
		{"url-encode", "URLEncode"},
		{"id", "ID"},
		{"uuid", "UUID"},
		{"foo-id-bar", "FooIDBar"},
	}
	for _, c := range cases {
		if got := GoName(c.in, true); got != c.want {
			t.Errorf("GoName(%q, true) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGoName_Unexported(t *testing.T) {
	cases := []struct{ in, want string }{
		{"add", "add"},
		{"get-value", "getValue"},
		{"http-status", "httpStatus"},  // first segment lowercase even if initialism
	}
	for _, c := range cases {
		if got := GoName(c.in, false); got != c.want {
			t.Errorf("GoName(%q, false) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGoPackageName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"calc", "calc"},
		{"calc-utils", "calcutils"},
		{"http-server", "httpserver"},
	}
	for _, c := range cases {
		if got := GoPackageName(c.in); got != c.want {
			t.Errorf("GoPackageName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTypeName(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{PrimU32, "U32"},
		{TypeString{}, "String"},
		{&TypeList{Elem: PrimU32}, "ListU32"},
		{&TypeList{Elem: TypeString{}}, "ListString"},
		{&TypeTuple{Fields: []Type{PrimU32, TypeString{}}}, "TupleU32String"},
		{&TypeTuple{Fields: []Type{
			&TypeList{Elem: TypeString{}},
			PrimU32,
		}}, "TupleListStringU32"},
	}
	for _, c := range cases {
		if got := TypeName(c.t); got != c.want {
			t.Errorf("TypeName(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestGoTypeOf(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{PrimU32, "uint32"},
		{TypeString{}, "string"},
		{&TypeList{Elem: PrimU32}, "[]uint32"},
		{&TypeList{Elem: TypeString{}}, "[]string"},
		{&TypeTuple{Fields: []Type{PrimU32, TypeString{}}}, "TupleU32String"},
		{&TypeTuple{Fields: []Type{
			&TypeList{Elem: TypeString{}},
			PrimU32,
		}}, "TupleListStringU32"},
	}
	for _, c := range cases {
		if got := GoTypeOf(c.t); got != c.want {
			t.Errorf("GoTypeOf(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestTypeName_NewTypes(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{&TypeOption{Elem: PrimU32}, "OptionU32"},
		{&TypeResult{OK: PrimU32, Err: TypeString{}}, "ResultU32String"},
		{&TypeResult{OK: PrimU32}, "ResultU32_"},
		{&TypeResult{Err: TypeString{}}, "Result_String"},
		{&TypeResult{}, "Result__"},
		{&TypeEnum{Name: "color", GoName: "Color"}, "Color"},
		{&TypeFlags{Name: "perms", GoName: "Perms"}, "Perms"},
	}
	for _, c := range cases {
		if got := TypeName(c.t); got != c.want {
			t.Errorf("TypeName(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestGoTypeOf_NewTypes(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{&TypeOption{Elem: PrimU32}, "OptionU32"},
		{&TypeResult{OK: PrimU32, Err: TypeString{}}, "ResultU32String"},
		{&TypeEnum{GoName: "Color"}, "Color"},
		{&TypeFlags{GoName: "Perms"}, "Perms"},
	}
	for _, c := range cases {
		if got := GoTypeOf(c.t); got != c.want {
			t.Errorf("GoTypeOf(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestTypeName_Record(t *testing.T) {
	rec := &TypeRecord{GoName: "Point"}
	if got := TypeName(rec); got != "Point" {
		t.Errorf("TypeName(record) = %q, want %q", got, "Point")
	}
}

func TestGoTypeOf_Record(t *testing.T) {
	rec := &TypeRecord{GoName: "Point"}
	if got := GoTypeOf(rec); got != "Point" {
		t.Errorf("GoTypeOf(record) = %q, want %q", got, "Point")
	}
}

func TestTypeName_Variant(t *testing.T) {
	v := &TypeVariant{GoName: "Shape"}
	if got := TypeName(v); got != "Shape" {
		t.Errorf("TypeName(variant) = %q, want %q", got, "Shape")
	}
}

func TestGoTypeOf_Variant(t *testing.T) {
	v := &TypeVariant{GoName: "Shape"}
	if got := GoTypeOf(v); got != "Shape" {
		t.Errorf("GoTypeOf(variant) = %q, want %q", got, "Shape")
	}
}

func TestTypeName_Resource(t *testing.T) {
	rt := &TypeResource{GoName: "Counter"}
	if got := TypeName(rt); got != "Counter" {
		t.Errorf("TypeName(resource) = %q, want %q", got, "Counter")
	}
	own := &TypeOwn{Resource: rt}
	if got := TypeName(own); got != "Counter" {
		t.Errorf("TypeName(own) = %q, want %q", got, "Counter")
	}
	borrow := &TypeBorrow{Resource: rt}
	if got := TypeName(borrow); got != "Counter" {
		t.Errorf("TypeName(borrow) = %q, want %q", got, "Counter")
	}
}

func TestGoTypeOf_Resource(t *testing.T) {
	rt := &TypeResource{GoName: "Counter", WrapName: "Counter"}
	if got := GoTypeOf(rt); got != "*CounterHandle" {
		t.Errorf("GoTypeOf(resource) = %q, want %q", got, "*CounterHandle")
	}
	own := &TypeOwn{Resource: rt}
	if got := GoTypeOf(own); got != "*CounterHandle" {
		t.Errorf("GoTypeOf(own) = %q, want %q", got, "*CounterHandle")
	}
	borrow := &TypeBorrow{Resource: rt}
	if got := GoTypeOf(borrow); got != "*CounterHandle" {
		t.Errorf("GoTypeOf(borrow) = %q, want %q", got, "*CounterHandle")
	}
}

func TestGoTypeOf_NominalQualified(t *testing.T) {
	cases := []struct {
		name string
		t    Type
		want string
	}{
		{"local-record", &TypeRecord{GoName: "Foo"}, "Foo"},
		{"qualified-record", &TypeRecord{GoName: "Foo", GoPackageQualifier: "streams"}, "streams.Foo"},
		{"local-variant", &TypeVariant{GoName: "Shape"}, "Shape"},
		{"qualified-variant", &TypeVariant{GoName: "Shape", GoPackageQualifier: "defs"}, "defs.Shape"},
		{"local-enum", &TypeEnum{GoName: "Color"}, "Color"},
		{"qualified-enum", &TypeEnum{GoName: "Color", GoPackageQualifier: "defs"}, "defs.Color"},
		{"local-flags", &TypeFlags{GoName: "Style"}, "Style"},
		{"qualified-flags", &TypeFlags{GoName: "Style", GoPackageQualifier: "defs"}, "defs.Style"},
	}
	for _, c := range cases {
		got := GoTypeOf(c.t)
		if got != c.want {
			t.Errorf("%s: GoTypeOf = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTypeName_QualifierPrefix(t *testing.T) {
	rec := &TypeRecord{GoName: "Foo", GoPackageQualifier: "streams"}
	enum := &TypeEnum{GoName: "Color", GoPackageQualifier: "defs"}
	cases := []struct {
		name string
		t    Type
		want string
	}{
		{"qualified-record", rec, "StreamsFoo"},
		{"option<qualified>", &TypeOption{Elem: rec}, "OptionStreamsFoo"},
		{"list<qualified>", &TypeList{Elem: rec}, "ListStreamsFoo"},
		{"qualified-enum", enum, "DefsColor"},
	}
	for _, c := range cases {
		got := TypeName(c.t)
		if got != c.want {
			t.Errorf("%s: TypeName = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestZeroValueExpr(t *testing.T) {
	cases := []struct {
		name string
		t    Type
		want string
	}{
		{"u32", PrimU32, "0"},
		{"string", TypeString{}, `""`},
		{"bool", PrimBool, "false"},
		{"f64", PrimF64, "0"},
		{"list", &TypeList{Elem: PrimU32}, "nil"},
		{"resource", &TypeResource{Name: "r", GoName: "R"}, "nil"},
		{"variant", &TypeVariant{Name: "shape-kind", GoName: "ShapeKind"}, "nil"},
		{"enum", &TypeEnum{Name: "color", GoName: "Color"}, "Color(0)"},
		{"flags", &TypeFlags{Name: "perms", GoName: "Perms"}, "Perms(0)"},
	}
	for _, tc := range cases {
		if got := ZeroValueExpr(tc.t); got != tc.want {
			t.Errorf("ZeroValueExpr(%s): got %q, want %q", tc.name, got, tc.want)
		}
	}
}
