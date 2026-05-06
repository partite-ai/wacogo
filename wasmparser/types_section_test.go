package wasmparser

import (
	"bytes"
	"testing"
)

// TestComponentImport_PlainFuncRef tests a component import with plain name and func type ref.
func TestComponentImport_PlainFuncRef(t *testing.T) {
	data := []byte{
		0x00,                          // plain name
		0x05, 'h', 'e', 'l', 'l', 'o', // "hello"
		0x01,                          // func type ref
		0x03,                          // type index 3
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	var imp ComponentImport
	if err := imp.unmarshalBinary(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if imp.Name.Kind != ImportNamePlain {
		t.Fatalf("name kind: got %d, want ImportNamePlain", imp.Name.Kind)
	}
	if imp.Name.Name != "hello" {
		t.Fatalf("name: got %q, want %q", imp.Name.Name, "hello")
	}
	tr, ok := imp.Type.(TypeRefFunc)
	if !ok {
		t.Fatalf("expected TypeRefFunc, got %T", imp.Type)
	}
	if tr.Index != 3 {
		t.Fatalf("type index: got %d, want 3", tr.Index)
	}
}

// TestComponentImport_TypeBoundsEq tests an import with type bounds eq.
func TestComponentImport_TypeBoundsEq(t *testing.T) {
	data := []byte{
		0x00,                    // plain name
		0x03, 'f', 'o', 'o',   // "foo"
		0x03,                    // type ref
		0x00,                    // eq bounds
		0x07,                    // index 7
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	var imp ComponentImport
	if err := imp.unmarshalBinary(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr, ok := imp.Type.(TypeRefType)
	if !ok {
		t.Fatalf("expected TypeRefType, got %T", imp.Type)
	}
	bounds, ok := tr.Bounds.(TypeBoundsEq)
	if !ok {
		t.Fatalf("expected TypeBoundsEq, got %T", tr.Bounds)
	}
	if bounds.Index != 7 {
		t.Fatalf("bounds index: got %d, want 7", bounds.Index)
	}
}

// TestComponentImport_TypeBoundsSubResource tests an import with sub-resource bounds.
func TestComponentImport_TypeBoundsSubResource(t *testing.T) {
	data := []byte{
		0x00,                    // plain name
		0x03, 'b', 'a', 'r',   // "bar"
		0x03,                    // type ref
		0x01,                    // sub resource bounds
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	var imp ComponentImport
	if err := imp.unmarshalBinary(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr, ok := imp.Type.(TypeRefType)
	if !ok {
		t.Fatalf("expected TypeRefType, got %T", imp.Type)
	}
	_, ok = tr.Bounds.(TypeBoundsSubResource)
	if !ok {
		t.Fatalf("expected TypeBoundsSubResource, got %T", tr.Bounds)
	}
}

// TestComponentExport_FuncNoAscription tests an export with func kind, no ascribed type.
func TestComponentExport_FuncNoAscription(t *testing.T) {
	data := []byte{
		0x00,                              // plain name
		0x05, 'h', 'e', 'l', 'l', 'o',   // "hello"
		0x01,                              // func kind
		0x02,                              // index 2
		0x00,                              // no ascribed type
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	var exp ComponentExport
	if err := exp.unmarshalBinary(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp.Name.Kind != ImportNamePlain {
		t.Fatalf("name kind: got %d, want ImportNamePlain", exp.Name.Kind)
	}
	if exp.Name.Name != "hello" {
		t.Fatalf("name: got %q, want %q", exp.Name.Name, "hello")
	}
	if exp.Kind != ExternalKindFunc {
		t.Fatalf("kind: got %d, want ExternalKindFunc", exp.Kind)
	}
	if exp.Index != 2 {
		t.Fatalf("index: got %d, want 2", exp.Index)
	}
	if exp.AscribedType.Valid {
		t.Fatal("expected no ascribed type")
	}
}

// TestComponentExport_WithAscribedType tests an export with an ascribed type.
func TestComponentExport_WithAscribedType(t *testing.T) {
	data := []byte{
		0x00,                    // plain name
		0x03, 'f', 'o', 'o',   // "foo"
		0x01,                    // func kind
		0x00,                    // index 0
		0x01,                    // has ascribed type (0x01 = yes)
		0x01,                    // type ref kind: func
		0x05,                    // type index 5
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	var exp ComponentExport
	if err := exp.unmarshalBinary(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exp.AscribedType.Valid {
		t.Fatal("expected ascribed type to be present")
	}
	tr, ok := exp.AscribedType.Value.(TypeRefFunc)
	if !ok {
		t.Fatalf("expected TypeRefFunc, got %T", exp.AscribedType.Value)
	}
	if tr.Index != 5 {
		t.Fatalf("ascribed type index: got %d, want 5", tr.Index)
	}
}

// TestAliasInstanceExport tests alias from a component instance export.
func TestAliasInstanceExport(t *testing.T) {
	data := []byte{
		0x01,                              // sort: func kind
		0x00,                              // discriminant: instance export alias
		0x03,                              // instance index 3
		0x05, 'h', 'e', 'l', 'l', 'o',   // name "hello"
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	alias, err := readComponentAlias(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := alias.(AliasInstanceExport)
	if !ok {
		t.Fatalf("expected AliasInstanceExport, got %T", alias)
	}
	if a.Kind != ExternalKindFunc {
		t.Fatalf("kind: got %d, want ExternalKindFunc", a.Kind)
	}
	if a.Instance != 3 {
		t.Fatalf("instance: got %d, want 3", a.Instance)
	}
	if a.Name != "hello" {
		t.Fatalf("name: got %q, want %q", a.Name, "hello")
	}
}

// TestAliasCoreInstanceExport tests alias from a core instance export.
func TestAliasCoreInstanceExport(t *testing.T) {
	data := []byte{
		0x00,                          // sort byte1: core prefix
		0x02,                          // sort byte2: memory
		0x01,                          // discriminant: core instance export alias
		0x01,                          // instance index 1
		0x06, 'm', 'e', 'm', 'o', 'r', 'y', // name "memory"
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	alias, err := readComponentAlias(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := alias.(AliasCoreInstanceExport)
	if !ok {
		t.Fatalf("expected AliasCoreInstanceExport, got %T", alias)
	}
	if a.Kind != CoreSortMemory {
		t.Fatalf("kind: got %d, want CoreSortMemory", a.Kind)
	}
	if a.Instance != 1 {
		t.Fatalf("instance: got %d, want 1", a.Instance)
	}
	if a.Name != "memory" {
		t.Fatalf("name: got %q, want %q", a.Name, "memory")
	}
}

// TestAliasOuter tests outer alias.
func TestAliasOuter(t *testing.T) {
	data := []byte{
		0x03, // sort: type kind
		0x02, // discriminant: outer alias
		0x01, // count 1
		0x04, // index 4
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	alias, err := readComponentAlias(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := alias.(AliasOuter)
	if !ok {
		t.Fatalf("expected AliasOuter, got %T", alias)
	}
	if a.Kind != OuterAliasKindType {
		t.Fatalf("kind: got %d, want OuterAliasKindType", a.Kind)
	}
	if a.Count != 1 {
		t.Fatalf("count: got %d, want 1", a.Count)
	}
	if a.Index != 4 {
		t.Fatalf("index: got %d, want 4", a.Index)
	}
}

// TestCanonLift_WithOptions tests canon lift with utf8 and memory options.
func TestCanonLift_WithOptions(t *testing.T) {
	data := []byte{
		0x00, // lift
		0x00, // core func flag
		0x02, // core func index 2
		0x02, // 2 options
		0x00, // utf8
		0x03, // memory
		0x00, // memory index 0
		0x07, // type index 7
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	cf, err := readCanonicalFunction(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lift, ok := cf.(CanonLift)
	if !ok {
		t.Fatalf("expected CanonLift, got %T", cf)
	}
	if lift.CoreFuncIndex != 2 {
		t.Fatalf("core func index: got %d, want 2", lift.CoreFuncIndex)
	}
	if lift.TypeIndex != 7 {
		t.Fatalf("type index: got %d, want 7", lift.TypeIndex)
	}
	if len(lift.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(lift.Options))
	}
	if _, ok := lift.Options[0].(CanonOptUTF8); !ok {
		t.Fatalf("option[0]: expected CanonOptUTF8, got %T", lift.Options[0])
	}
	mem, ok := lift.Options[1].(CanonOptMemory)
	if !ok {
		t.Fatalf("option[1]: expected CanonOptMemory, got %T", lift.Options[1])
	}
	if mem.Index != 0 {
		t.Fatalf("memory index: got %d, want 0", mem.Index)
	}
}

// TestCanonLower_Basic tests a basic canon lower.
func TestCanonLower_Basic(t *testing.T) {
	data := []byte{
		0x01, // lower
		0x00, // sub-type byte
		0x03, // func index 3
		0x00, // 0 options
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	cf, err := readCanonicalFunction(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lower, ok := cf.(CanonLower)
	if !ok {
		t.Fatalf("expected CanonLower, got %T", cf)
	}
	if lower.FuncIndex != 3 {
		t.Fatalf("func index: got %d, want 3", lower.FuncIndex)
	}
	if len(lower.Options) != 0 {
		t.Fatalf("expected 0 options, got %d", len(lower.Options))
	}
}

// TestCanonResourceNew tests resource.new.
func TestCanonResourceNew(t *testing.T) {
	data := []byte{0x02, 0x01} // resource.new type index 1
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	cf, err := readCanonicalFunction(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rn, ok := cf.(CanonResourceNew)
	if !ok {
		t.Fatalf("expected CanonResourceNew, got %T", cf)
	}
	if rn.TypeIndex != 1 {
		t.Fatalf("type index: got %d, want 1", rn.TypeIndex)
	}
}

// TestCanonResourceDrop tests resource.drop.
func TestCanonResourceDrop(t *testing.T) {
	data := []byte{0x03, 0x02} // resource.drop type index 2
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	cf, err := readCanonicalFunction(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rd, ok := cf.(CanonResourceDrop)
	if !ok {
		t.Fatalf("expected CanonResourceDrop, got %T", cf)
	}
	if rd.TypeIndex != 2 {
		t.Fatalf("type index: got %d, want 2", rd.TypeIndex)
	}
}

// TestComponentInstance_Instantiate tests instantiating a component with args.
func TestComponentInstance_Instantiate(t *testing.T) {
	data := []byte{
		0x00,                              // instantiate
		0x01,                              // component index 1
		0x01,                              // 1 arg
		0x05, 'h', 'e', 'l', 'l', 'o',   // arg name "hello"
		0x01,                              // func kind
		0x03,                              // index 3
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	ci, err := readComponentInstance(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inst, ok := ci.(Instantiate)
	if !ok {
		t.Fatalf("expected Instantiate, got %T", ci)
	}
	if inst.ComponentIndex != 1 {
		t.Fatalf("component index: got %d, want 1", inst.ComponentIndex)
	}
	if len(inst.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(inst.Args))
	}
	if inst.Args[0].Name != "hello" {
		t.Fatalf("arg name: got %q, want %q", inst.Args[0].Name, "hello")
	}
	if inst.Args[0].Kind != ExternalKindFunc {
		t.Fatalf("arg kind: got %d, want ExternalKindFunc", inst.Args[0].Kind)
	}
	if inst.Args[0].Index != 3 {
		t.Fatalf("arg index: got %d, want 3", inst.Args[0].Index)
	}
}

// TestCoreInstance_Instantiate tests instantiating a core module.
func TestCoreInstance_Instantiate(t *testing.T) {
	data := []byte{
		0x00,                              // instantiate
		0x00,                              // module index 0
		0x01,                              // 1 arg
		0x03, 'e', 'n', 'v',              // arg name "env"
		0x12,                              // instance sort
		0x01,                              // index 1
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	ci, err := readInstance(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inst, ok := ci.(CoreInstantiate)
	if !ok {
		t.Fatalf("expected CoreInstantiate, got %T", ci)
	}
	if inst.ModuleIndex != 0 {
		t.Fatalf("module index: got %d, want 0", inst.ModuleIndex)
	}
	if len(inst.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(inst.Args))
	}
	if inst.Args[0].Name != "env" {
		t.Fatalf("arg name: got %q, want %q", inst.Args[0].Name, "env")
	}
	if inst.Args[0].Kind != CoreSortInstance {
		t.Fatalf("arg kind: got %d, want CoreSortInstance", inst.Args[0].Kind)
	}
	if inst.Args[0].Index != 1 {
		t.Fatalf("arg index: got %d, want 1", inst.Args[0].Index)
	}
}

// TestComponentStartFunction tests decoding a start function.
func TestComponentStartFunction(t *testing.T) {
	data := []byte{
		0x02,       // func index 2
		0x02,       // 2 args
		0x00, 0x01, // arg 0, arg 1
		0x01,       // 1 result
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	var sf ComponentStartFunction
	if err := sf.unmarshalBinary(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sf.FuncIndex != 2 {
		t.Fatalf("func index: got %d, want 2", sf.FuncIndex)
	}
	if len(sf.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(sf.Args))
	}
	if sf.Args[0] != 0 || sf.Args[1] != 1 {
		t.Fatalf("args: got %v, want [0, 1]", sf.Args)
	}
	if sf.Results != 1 {
		t.Fatalf("results: got %d, want 1", sf.Results)
	}
}

// TestReadComponentExternalKind tests all external kind encodings.
func TestReadComponentExternalKind(t *testing.T) {
	tests := []struct {
		data []byte
		want ComponentExternalKind
	}{
		{[]byte{0x00, 0x11}, ExternalKindModule},
		{[]byte{0x01}, ExternalKindFunc},
		{[]byte{0x02}, ExternalKindValue},
		{[]byte{0x03}, ExternalKindType},
		{[]byte{0x04}, ExternalKindComponent},
		{[]byte{0x05}, ExternalKindInstance},
	}
	for _, tt := range tests {
		r := newBinaryReaderAt(bytes.NewReader(tt.data), 0)
		kind, err := readComponentExternalKind(r)
		if err != nil {
			t.Fatalf("data=%x: unexpected error: %v", tt.data, err)
		}
		if kind != tt.want {
			t.Fatalf("data=%x: got %d, want %d", tt.data, kind, tt.want)
		}
	}
}

// TestReadCoreSort tests all core sort encodings.
func TestReadCoreSort(t *testing.T) {
	tests := []struct {
		data byte
		want CoreSort
	}{
		{0x00, CoreSortFunc},
		{0x01, CoreSortTable},
		{0x02, CoreSortMemory},
		{0x03, CoreSortGlobal},
		{0x10, CoreSortType},
		{0x11, CoreSortModule},
		{0x12, CoreSortInstance},
	}
	for _, tt := range tests {
		r := newBinaryReaderAt(bytes.NewReader([]byte{tt.data}), 0)
		sort, err := readCoreSort(r)
		if err != nil {
			t.Fatalf("byte=0x%02x: unexpected error: %v", tt.data, err)
		}
		if sort != tt.want {
			t.Fatalf("byte=0x%02x: got %d, want %d", tt.data, sort, tt.want)
		}
	}
}
