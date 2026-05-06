package witgen

import (
	"strings"
	"testing"
)

func TestLowerWorld_AddInterface(t *testing.T) {
	res := loadCached(t, "testdata/wit/add.wit")
	pkg, err := LowerWorld(res, "example:demo/arith")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	if len(pkg.Interfaces) != 1 {
		t.Fatalf("Interfaces: got %d, want 1", len(pkg.Interfaces))
	}
	iface := pkg.Interfaces[0]
	if iface.GoName != "Calc" {
		t.Errorf("iface.GoName: got %q, want %q", iface.GoName, "Calc")
	}
	if iface.GoPackage != "calc" {
		t.Errorf("iface.GoPackage: got %q, want %q", iface.GoPackage, "calc")
	}
	if iface.Namespace != "example" || iface.Package != "demo" || iface.Name != "calc" {
		t.Errorf("iface namespace/package/name: got %q/%q/%q",
			iface.Namespace, iface.Package, iface.Name)
	}
	if len(iface.Funcs) != 1 {
		t.Fatalf("Funcs: got %d, want 1", len(iface.Funcs))
	}
	fn := iface.Funcs[0]
	if fn.WitName != "add" || fn.GoName != "Add" {
		t.Errorf("fn name: got %q/%q, want add/Add", fn.WitName, fn.GoName)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("Params: got %d, want 2", len(fn.Params))
	}
	if fn.Params[0].GoName != "a" || fn.Params[0].Type != PrimU32 {
		t.Errorf("param 0: got %q/%v, want a/PrimU32", fn.Params[0].GoName, fn.Params[0].Type)
	}
	if fn.Params[1].GoName != "b" || fn.Params[1].Type != PrimU32 {
		t.Errorf("param 1: got %q/%v", fn.Params[1].GoName, fn.Params[1].Type)
	}
	if got, ok := fn.Result.(Prim); !ok || got != PrimU32 {
		t.Errorf("Result: got %v, want PrimU32", fn.Result)
	}
}

func TestLowerWorld_StructuralCompounds(t *testing.T) {
	res := loadCached(t, "testdata/wit/types_compound.wit")
	pkg, err := LowerWorld(res, "example:demo/combo")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	if len(pkg.Interfaces) != 1 {
		t.Fatalf("Interfaces: got %d, want 1", len(pkg.Interfaces))
	}
	iface := pkg.Interfaces[0]
	if len(iface.Funcs) != 4 {
		t.Fatalf("Funcs: got %d, want 4", len(iface.Funcs))
	}

	funcs := map[string]*Func{}
	for _, f := range iface.Funcs {
		funcs[f.GoName] = f
	}

	// greet: (name: string) -> string
	if g := funcs["Greet"]; g == nil {
		t.Fatal("missing Greet")
	} else {
		if _, ok := g.Params[0].Type.(TypeString); !ok {
			t.Errorf("Greet param: want TypeString, got %T", g.Params[0].Type)
		}
		if _, ok := g.Result.(TypeString); !ok {
			t.Errorf("Greet result: want TypeString, got %T", g.Result)
		}
	}

	// sum: (xs: list<u32>) -> u32
	if s := funcs["Sum"]; s == nil {
		t.Fatal("missing Sum")
	} else {
		lst, ok := s.Params[0].Type.(*TypeList)
		if !ok {
			t.Fatalf("Sum param: want *TypeList, got %T", s.Params[0].Type)
		}
		if elem, ok := lst.Elem.(Prim); !ok || elem != PrimU32 {
			t.Errorf("Sum list elem: want PrimU32, got %v", lst.Elem)
		}
	}

	// pair: () -> tuple<u32, u32>
	if p := funcs["Pair"]; p == nil {
		t.Fatal("missing Pair")
	} else {
		tup, ok := p.Result.(*TypeTuple)
		if !ok {
			t.Fatalf("Pair result: want *TypeTuple, got %T", p.Result)
		}
		if len(tup.Fields) != 2 {
			t.Fatalf("Pair tuple fields: want 2, got %d", len(tup.Fields))
		}
		for i, f := range tup.Fields {
			if prim, ok := f.(Prim); !ok || prim != PrimU32 {
				t.Errorf("Pair tuple[%d]: want PrimU32, got %v", i, f)
			}
		}
	}

	// nested: () -> list<tuple<string, u32>>
	if n := funcs["Nested"]; n == nil {
		t.Fatal("missing Nested")
	} else {
		lst, ok := n.Result.(*TypeList)
		if !ok {
			t.Fatalf("Nested result: want *TypeList, got %T", n.Result)
		}
		tup, ok := lst.Elem.(*TypeTuple)
		if !ok {
			t.Fatalf("Nested elem: want *TypeTuple, got %T", lst.Elem)
		}
		if len(tup.Fields) != 2 {
			t.Fatalf("Nested tuple: want 2 fields, got %d", len(tup.Fields))
		}
		if _, ok := tup.Fields[0].(TypeString); !ok {
			t.Errorf("Nested tuple[0]: want TypeString, got %T", tup.Fields[0])
		}
		if prim, ok := tup.Fields[1].(Prim); !ok || prim != PrimU32 {
			t.Errorf("Nested tuple[1]: want PrimU32, got %v", tup.Fields[1])
		}
	}
}

func TestLowerWorld_OptionResult(t *testing.T) {
	res := loadCached(t, "testdata/wit/option_result.wit")
	pkg, err := LowerWorld(res, "example:demo/tryhost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	iface := pkg.Interfaces[0]

	funcs := map[string]*Func{}
	for _, f := range iface.Funcs {
		funcs[f.GoName] = f
	}

	// find: -> option<string>
	find := funcs["Find"]
	if find == nil {
		t.Fatal("missing Find")
	}
	opt, ok := find.Result.(*TypeOption)
	if !ok {
		t.Fatalf("Find.Result: want *TypeOption, got %T", find.Result)
	}
	if _, ok := opt.Elem.(TypeString); !ok {
		t.Errorf("option elem: want TypeString, got %T", opt.Elem)
	}

	// parse: -> result<u32, string>
	parse := funcs["Parse"]
	if parse == nil {
		t.Fatal("missing Parse")
	}
	r, ok := parse.Result.(*TypeResult)
	if !ok {
		t.Fatalf("Parse.Result: want *TypeResult, got %T", parse.Result)
	}
	if prim, ok := r.OK.(Prim); !ok || prim != PrimU32 {
		t.Errorf("result OK: want PrimU32, got %v", r.OK)
	}
	if _, ok := r.Err.(TypeString); !ok {
		t.Errorf("result Err: want TypeString, got %T", r.Err)
	}
}

func TestLowerWorld_EnumFlags(t *testing.T) {
	res := loadCached(t, "testdata/wit/enum_flags.wit")
	pkg, err := LowerWorld(res, "example:demo/efhost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	iface := pkg.Interfaces[0]

	if len(iface.Enums) != 1 {
		t.Fatalf("Enums: got %d, want 1", len(iface.Enums))
	}
	dir := iface.Enums[0]
	if dir.Name != "direction" || dir.GoName != "Direction" {
		t.Errorf("enum name: got %q/%q", dir.Name, dir.GoName)
	}
	if len(dir.Cases) != 4 {
		t.Fatalf("enum cases: got %d, want 4", len(dir.Cases))
	}
	// spot-check first case:
	if dir.Cases[0].Name != "north" || dir.Cases[0].GoName != "DirectionNorth" {
		t.Errorf("enum case 0: got %q/%q", dir.Cases[0].Name, dir.Cases[0].GoName)
	}

	if len(iface.Flags) != 1 {
		t.Fatalf("Flags: got %d", len(iface.Flags))
	}
	perms := iface.Flags[0]
	if perms.GoName != "Perms" {
		t.Errorf("flags name: got %q", perms.GoName)
	}
	if len(perms.Cases) != 3 {
		t.Fatalf("flag cases: got %d, want 3", len(perms.Cases))
	}

	// go-way: (d: direction) -> direction
	// POINTER identity: the param/result should be the SAME *TypeEnum
	// that iface.Enums[0] points to.
	funcs := map[string]*Func{}
	for _, f := range iface.Funcs {
		funcs[f.GoName] = f
	}
	gw := funcs["GoWay"]
	if gw == nil {
		t.Fatal("missing GoWay")
	}
	paramType, ok := gw.Params[0].Type.(*TypeEnum)
	if !ok {
		t.Fatalf("GoWay param: want *TypeEnum, got %T", gw.Params[0].Type)
	}
	if paramType != dir {
		t.Errorf("GoWay param: want pointer-equal to iface.Enums[0]; got different pointer")
	}
	resultType, ok := gw.Result.(*TypeEnum)
	if !ok {
		t.Fatalf("GoWay result: want *TypeEnum, got %T", gw.Result)
	}
	if resultType != dir {
		t.Errorf("GoWay result: want pointer-equal to iface.Enums[0]")
	}

	// check: (p: perms) -> bool
	ch := funcs["Check"]
	if ch == nil {
		t.Fatal("missing Check")
	}
	paramFlags, ok := ch.Params[0].Type.(*TypeFlags)
	if !ok {
		t.Fatalf("Check param: want *TypeFlags, got %T", ch.Params[0].Type)
	}
	if paramFlags != perms {
		t.Errorf("Check param: want pointer-equal to iface.Flags[0]")
	}
}

func TestLowerWorld_Record(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_record_only.wit")
	pkg, err := LowerWorld(res, "example:demo/pointshost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	iface := pkg.Interfaces[0]

	if len(iface.Records) != 1 {
		t.Fatalf("Records: got %d, want 1", len(iface.Records))
	}
	rec := iface.Records[0]
	if rec.Name != "point" {
		t.Errorf("Name: got %q, want point", rec.Name)
	}
	if rec.GoName != "Point" {
		t.Errorf("GoName: got %q, want Point", rec.GoName)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("Fields: got %d, want 2", len(rec.Fields))
	}
	if rec.Fields[0].GoName != "X" || rec.Fields[1].GoName != "Y" {
		t.Errorf("field GoNames: got %q,%q", rec.Fields[0].GoName, rec.Fields[1].GoName)
	}
	if p, ok := rec.Fields[0].Type.(Prim); !ok || p != PrimU32 {
		t.Errorf("field 0 type: %v", rec.Fields[0].Type)
	}

	// Function signature must reference the SAME *TypeRecord pointer.
	fn := iface.Funcs[0]
	p0, ok := fn.Params[0].Type.(*TypeRecord)
	if !ok {
		t.Fatalf("midpoint param 0: want *TypeRecord, got %T", fn.Params[0].Type)
	}
	if p0 != rec {
		t.Errorf("midpoint param 0 not pointer-equal to iface.Records[0]")
	}
	if r, ok := fn.Result.(*TypeRecord); !ok || r != rec {
		t.Errorf("midpoint result not pointer-equal to iface.Records[0]")
	}
}

func TestLowerWorld_Variant(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_variant_only.wit")
	pkg, err := LowerWorld(res, "example:demo/shapeshost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	iface := pkg.Interfaces[0]

	if len(iface.Variants) != 1 {
		t.Fatalf("Variants: got %d, want 1", len(iface.Variants))
	}
	v := iface.Variants[0]
	if v.Name != "shape-kind" || v.GoName != "ShapeKind" {
		t.Errorf("name: got %q/%q", v.Name, v.GoName)
	}
	if len(v.Cases) != 3 {
		t.Fatalf("Cases: got %d, want 3", len(v.Cases))
	}
	// circle has u32 payload
	if v.Cases[0].Name != "circle" || v.Cases[0].GoName != "ShapeKindCircle" {
		t.Errorf("case 0: got %q/%q", v.Cases[0].Name, v.Cases[0].GoName)
	}
	if p, ok := v.Cases[0].Payload.(Prim); !ok || p != PrimU32 {
		t.Errorf("case 0 payload: got %v, want PrimU32", v.Cases[0].Payload)
	}
	// rect has point payload (POINTER-equal to iface.Records[0])
	if len(iface.Records) != 1 {
		t.Fatalf("expected one record (point)")
	}
	rec := iface.Records[0]
	if r, ok := v.Cases[1].Payload.(*TypeRecord); !ok || r != rec {
		t.Errorf("case 1 payload: not pointer-equal to iface.Records[0]")
	}
	// none has nil payload
	if v.Cases[2].Payload != nil {
		t.Errorf("case 2 payload: got %v, want nil", v.Cases[2].Payload)
	}

	// Func sig points at the same *TypeVariant.
	fn := iface.Funcs[0]
	if vt, ok := fn.Params[0].Type.(*TypeVariant); !ok || vt != v {
		t.Errorf("name-of param 0: not pointer-equal to iface.Variants[0]")
	}
}

func TestLowerWorld_Resource(t *testing.T) {
	res := loadCached(t, "testdata/wit/has_counter.wit")
	pkg, err := LowerWorld(res, "example:demo/countershost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	iface := pkg.Interfaces[0]

	if len(iface.Resources) != 1 {
		t.Fatalf("Resources: got %d, want 1", len(iface.Resources))
	}
	rt := iface.Resources[0]
	if rt.Name != "counter" || rt.GoName != "Counter" {
		t.Errorf("name: %q/%q", rt.Name, rt.GoName)
	}
	if rt.Ctor == nil {
		t.Fatal("ctor missing")
	}
	if len(rt.Ctor.Params) != 1 {
		t.Fatalf("ctor params: %d", len(rt.Ctor.Params))
	}
	if rt.Ctor.Params[0].GoName != "initial" {
		t.Errorf("ctor param 0 GoName: %q", rt.Ctor.Params[0].GoName)
	}
	if len(rt.Methods) != 2 {
		t.Fatalf("Methods: got %d, want 2", len(rt.Methods))
	}
	methods := map[string]ResourceMethod{}
	for _, m := range rt.Methods {
		methods[m.Name] = m
	}
	if _, ok := methods["increment"]; !ok {
		t.Error("missing increment")
	}
	if cur, ok := methods["current"]; !ok {
		t.Error("missing current")
	} else if p, ok := cur.Result.(Prim); !ok || p != PrimU32 {
		t.Errorf("current result: got %v, want PrimU32", cur.Result)
	}

	// Functions list should be empty (all of counter's funcs are
	// resource methods/ctor/static, not freestanding).
	if len(iface.Funcs) != 0 {
		t.Errorf("freestanding Funcs: got %d, want 0", len(iface.Funcs))
	}
}

func TestLowerInterface_ImportsCaptured(t *testing.T) {
	res := loadCached(t, "testdata/wit/cross_resource.wit")
	pkg, err := LowerWorld(res, "example:demo/crossres")
	if err != nil {
		t.Fatal(err)
	}

	var fs *Interface
	var streams *Interface
	for _, iface := range pkg.Interfaces {
		switch iface.Name {
		case "filesystem":
			fs = iface
		case "streams":
			streams = iface
		}
	}
	if fs == nil {
		t.Fatal("filesystem not found")
	}
	if streams == nil {
		t.Fatal("streams not found")
	}

	if len(streams.Imports) != 0 {
		t.Errorf("streams should have no imports, got %d", len(streams.Imports))
	}
	if len(fs.Imports) != 1 {
		t.Fatalf("filesystem should have 1 import, got %d", len(fs.Imports))
	}
	imp := fs.Imports[0]
	if imp.Name != "streams" {
		t.Errorf("import name: got %q, want streams", imp.Name)
	}
	if imp.GoPackage != "streams" {
		t.Errorf("import GoPackage: got %q, want streams", imp.GoPackage)
	}
	if imp.GoFieldName != "Streams" {
		t.Errorf("import GoFieldName: got %q, want Streams", imp.GoFieldName)
	}
	if imp.StateField != "streams" {
		t.Errorf("import StateField: got %q, want streams", imp.StateField)
	}
	if len(imp.Resources) != 1 {
		t.Fatalf("import should reference 1 resource, got %d", len(imp.Resources))
	}
	if imp.Resources[0].Name != "handle" {
		t.Errorf("imported resource Name: got %q, want handle", imp.Resources[0].Name)
	}
	if imp.Resources[0].WrapName != "Handle" {
		t.Errorf("imported resource WrapName: got %q, want Handle", imp.Resources[0].WrapName)
	}
	if imp.Resources[0].RefField != "HandleRef" {
		t.Errorf("imported resource RefField: got %q, want HandleRef", imp.Resources[0].RefField)
	}
}

func TestLowerWorld_NominalOwnerInterfaceSet(t *testing.T) {
	cases := []struct {
		witPath string
		world   string
		// kinds to expect non-empty in any one of the package's interfaces;
		// any kind named here must appear at least once across iface.Records etc.
		expectRecords  bool
		expectVariants bool
		expectEnums    bool
		expectFlags    bool
	}{
		{"testdata/wit/shape.wit", "example:demo/shapehost", true, true, false, false},
		{"testdata/wit/enum_flags.wit", "example:demo/efhost", false, false, true, true},
	}
	for _, c := range cases {
		t.Run(c.witPath, func(t *testing.T) {
			res := loadCached(t, c.witPath)
			pkg, err := LowerWorld(res, c.world)
			if err != nil {
				t.Fatalf("LowerWorld: %v", err)
			}
			var sawRec, sawVar, sawEnum, sawFlags bool
			for _, iface := range pkg.Interfaces {
				for _, r := range iface.Records {
					sawRec = true
					if r.OwnerInterface != iface {
						t.Errorf("record %q OwnerInterface = %v, want %v", r.Name, r.OwnerInterface, iface)
					}
				}
				for _, v := range iface.Variants {
					sawVar = true
					if v.OwnerInterface != iface {
						t.Errorf("variant %q OwnerInterface = %v, want %v", v.Name, v.OwnerInterface, iface)
					}
				}
				for _, e := range iface.Enums {
					sawEnum = true
					if e.OwnerInterface != iface {
						t.Errorf("enum %q OwnerInterface = %v, want %v", e.Name, e.OwnerInterface, iface)
					}
				}
				for _, f := range iface.Flags {
					sawFlags = true
					if f.OwnerInterface != iface {
						t.Errorf("flags %q OwnerInterface = %v, want %v", f.Name, f.OwnerInterface, iface)
					}
				}
			}
			if c.expectRecords && !sawRec {
				t.Error("expected at least one record in fixture")
			}
			if c.expectVariants && !sawVar {
				t.Error("expected at least one variant in fixture")
			}
			if c.expectEnums && !sawEnum {
				t.Error("expected at least one enum in fixture")
			}
			if c.expectFlags && !sawFlags {
				t.Error("expected at least one flags in fixture")
			}
		})
	}
}

// TestCollectImports_NominalsCollected exercises the to-be-generalised
// walker. It loads a fabricated *Package by hand (no real WIT needed) so
// the test is self-contained and stable.
func TestCollectImports_NominalsCollected(t *testing.T) {
	defs := &Interface{
		Namespace: "x", Package: "y", Name: "defs",
		GoName: "Defs", WrapName: "Defs", ImplName: "DefsImpl", GoPackage: "defs",
	}
	point := &TypeRecord{Name: "point", GoName: "Point", OwnerInterface: defs,
		Fields: []RecordField{{Name: "x", GoName: "X", Type: PrimU32}, {Name: "y", GoName: "Y", Type: PrimU32}}}
	color := &TypeEnum{Name: "color", GoName: "Color", OwnerInterface: defs,
		Cases: []EnumCase{{Name: "red", GoName: "ColorRed"}}}
	defs.Records = append(defs.Records, point)
	defs.Enums = append(defs.Enums, color)

	uses := &Interface{
		Namespace: "x", Package: "y", Name: "uses",
		GoName: "Uses", WrapName: "Uses", ImplName: "UsesImpl", GoPackage: "uses",
	}
	uses.Funcs = []*Func{{
		WitName: "paint", GoName: "Paint",
		Params: []*Param{{GoName: "p", Type: point}, {GoName: "c", Type: color}},
		Result: &TypeOption{Elem: point}, // exercises compound recursion
	}}

	all := []*Interface{defs, uses}
	collectImports(uses, all, "")

	if len(uses.Imports) != 1 {
		t.Fatalf("Imports: got %d, want 1", len(uses.Imports))
	}
	imp := uses.Imports[0]
	if imp.GoPackage != "defs" {
		t.Errorf("GoPackage: got %q, want %q", imp.GoPackage, "defs")
	}
	if len(imp.Records) != 1 || imp.Records[0].Name != "point" {
		t.Errorf("Records: got %+v, want one entry named point", imp.Records)
	}
	if len(imp.Enums) != 1 || imp.Enums[0].Name != "color" {
		t.Errorf("Enums: got %+v, want one entry named color", imp.Enums)
	}
}

func TestRewriteImports_QualifierTaggedCopies(t *testing.T) {
	defs := &Interface{
		Namespace: "x", Package: "y", Name: "defs",
		GoName: "Defs", WrapName: "Defs", ImplName: "DefsImpl", GoPackage: "defs",
	}
	point := &TypeRecord{Name: "point", GoName: "Point", OwnerInterface: defs,
		Fields: []RecordField{{Name: "x", GoName: "X", Type: PrimU32}}}
	color := &TypeEnum{Name: "color", GoName: "Color", OwnerInterface: defs,
		Cases: []EnumCase{{Name: "red", GoName: "ColorRed"}}}
	defs.Records = append(defs.Records, point)
	defs.Enums = append(defs.Enums, color)

	uses := &Interface{
		Namespace: "x", Package: "y", Name: "uses",
		GoName: "Uses", WrapName: "Uses", ImplName: "UsesImpl", GoPackage: "uses",
	}
	uses.Funcs = []*Func{{
		WitName: "paint", GoName: "Paint",
		Params: []*Param{{GoName: "p", Type: point}, {GoName: "c", Type: color}},
		Result: &TypeOption{Elem: point}, // option-of-imported-record
	}}

	collectImports(uses, []*Interface{defs, uses}, "")
	rewriteImports(uses)

	cp := uses.Funcs[0].Params[0].Type.(*TypeRecord)
	if cp == point {
		t.Fatal("rewriteImports must shallow-copy the imported record, not reuse the original pointer")
	}
	if cp.GoPackageQualifier != "defs" {
		t.Errorf("copy GoPackageQualifier: got %q, want %q", cp.GoPackageQualifier, "defs")
	}
	if point.GoPackageQualifier != "" {
		t.Errorf("original GoPackageQualifier mutated: got %q, want empty", point.GoPackageQualifier)
	}

	resOpt := uses.Funcs[0].Result.(*TypeOption)
	resCp, ok := resOpt.Elem.(*TypeRecord)
	if !ok {
		t.Fatalf("Result.Elem not *TypeRecord: %T", resOpt.Elem)
	}
	if resCp != cp {
		t.Error("rewriteImports must reuse the per-iface copy across all references to the same imported nominal")
	}
	if resOpt.GoPackageQualifier != "" {
		t.Errorf("option in func signature should NOT be tagged: got qualifier %q", resOpt.GoPackageQualifier)
	}

	ec := uses.Funcs[0].Params[1].Type.(*TypeEnum)
	if ec.GoPackageQualifier != "defs" || ec == color {
		t.Errorf("enum copy not tagged correctly: %+v", ec)
	}
}

func TestLowerInterface_NameSplit(t *testing.T) {
	res := loadCached(t, "testdata/wit/add.wit")
	pkg, err := LowerWorld(res, "example:demo/arith")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	if len(pkg.Interfaces) == 0 {
		t.Fatal("expected at least one interface")
	}
	iface := pkg.Interfaces[0]
	if iface.WrapName == "" {
		t.Errorf("WrapName empty for %q", iface.Name)
	}
	if iface.WrapName != "Calc" {
		t.Errorf("WrapName: got %q, want Calc", iface.WrapName)
	}
	if iface.ImplName != iface.WrapName+"Impl" {
		t.Errorf("ImplName %q should equal WrapName %q + Impl", iface.ImplName, iface.WrapName)
	}
	if iface.GoName != iface.WrapName {
		t.Errorf("GoName %q should equal WrapName %q (back-compat)", iface.GoName, iface.WrapName)
	}
}

func TestLowerWorld_Docs(t *testing.T) {
	res := loadCached(t, "testdata/wit/docs.wit")
	pkg, err := LowerWorld(res, "example:demo/docshost")
	if err != nil {
		t.Fatalf("LowerWorld: %v", err)
	}
	if len(pkg.Interfaces) != 1 {
		t.Fatalf("Interfaces: got %d, want 1", len(pkg.Interfaces))
	}
	iface := pkg.Interfaces[0]

	if !strings.Contains(iface.Docs, "Package-level documentation") {
		t.Errorf("iface.Docs missing expected text: %q", iface.Docs)
	}

	// Freestanding function.
	var greet *Func
	for _, f := range iface.Funcs {
		if f.WitName == "greet" {
			greet = f
		}
	}
	if greet == nil {
		t.Fatalf("greet func not found")
	}
	if !strings.Contains(greet.Docs, "documented freestanding function") {
		t.Errorf("greet.Docs: %q", greet.Docs)
	}

	// Record + fields.
	if len(iface.Records) != 1 {
		t.Fatalf("Records: got %d", len(iface.Records))
	}
	rec := iface.Records[0]
	if !strings.Contains(rec.Docs, "documented record") {
		t.Errorf("rec.Docs: %q", rec.Docs)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("rec.Fields: got %d", len(rec.Fields))
	}
	if !strings.Contains(rec.Fields[0].Docs, "horizontal") {
		t.Errorf("Fields[0].Docs: %q", rec.Fields[0].Docs)
	}
	if !strings.Contains(rec.Fields[1].Docs, "vertical") {
		t.Errorf("Fields[1].Docs: %q", rec.Fields[1].Docs)
	}

	// Enum + cases.
	if len(iface.Enums) != 1 {
		t.Fatalf("Enums: got %d", len(iface.Enums))
	}
	en := iface.Enums[0]
	if !strings.Contains(en.Docs, "documented enum") {
		t.Errorf("en.Docs: %q", en.Docs)
	}
	if !strings.Contains(en.Cases[0].Docs, "Pointing up") {
		t.Errorf("Cases[0].Docs: %q", en.Cases[0].Docs)
	}

	// Flags + cases.
	if len(iface.Flags) != 1 {
		t.Fatalf("Flags: got %d", len(iface.Flags))
	}
	fl := iface.Flags[0]
	if !strings.Contains(fl.Docs, "documented flags set") {
		t.Errorf("fl.Docs: %q", fl.Docs)
	}
	if !strings.Contains(fl.Cases[0].Docs, "Read access") {
		t.Errorf("Cases[0].Docs: %q", fl.Cases[0].Docs)
	}

	// Variant + cases.
	if len(iface.Variants) != 1 {
		t.Fatalf("Variants: got %d", len(iface.Variants))
	}
	v := iface.Variants[0]
	if !strings.Contains(v.Docs, "documented variant") {
		t.Errorf("v.Docs: %q", v.Docs)
	}
	if !strings.Contains(v.Cases[0].Docs, "circle") {
		t.Errorf("Cases[0].Docs: %q", v.Cases[0].Docs)
	}
	if !strings.Contains(v.Cases[1].Docs, "square") {
		t.Errorf("Cases[1].Docs: %q", v.Cases[1].Docs)
	}

	// Resource + ctor + method + static.
	if len(iface.Resources) != 1 {
		t.Fatalf("Resources: got %d", len(iface.Resources))
	}
	r := iface.Resources[0]
	if !strings.Contains(r.Docs, "documented resource") {
		t.Errorf("r.Docs: %q", r.Docs)
	}
	if r.Ctor == nil || !strings.Contains(r.Ctor.Docs, "starting at zero") {
		t.Errorf("ctor.Docs: %v", r.Ctor)
	}
	var incrMethod *ResourceMethod
	for i := range r.Methods {
		if r.Methods[i].Name == "increment" {
			incrMethod = &r.Methods[i]
		}
	}
	if incrMethod == nil || !strings.Contains(incrMethod.Docs, "Increment the counter") {
		var doc string
		if incrMethod != nil {
			doc = incrMethod.Docs
		}
		t.Errorf("increment method Docs: %q", doc)
	}
	if len(r.Statics) == 0 || !strings.Contains(r.Statics[0].Docs, "starting at one") {
		t.Errorf("Statics[0].Docs: %q", r.Statics[0].Docs)
	}
}
