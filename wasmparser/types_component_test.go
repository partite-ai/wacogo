package wasmparser

import (
	"bytes"
	"testing"
)

// TestComponentFuncType_SingleResult tests a func type with 1 param and single unnamed result.
// Encoding: 0x40 (sync func), 1 param ("x", u32=0x79), result tag 0x00, string=0x73
func TestComponentFuncType_SingleResult(t *testing.T) {
	data := []byte{
		0x40,                    // sync func type
		0x01,                    // 1 param
		0x01, 'x',              // param name "x"
		0x79,                    // u32
		0x00,                    // single unnamed result
		0x73,                    // string
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	ct, err := readComponentTypeDef(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ft, ok := ct.(*ComponentFuncType)
	if !ok {
		t.Fatalf("expected *ComponentFuncType, got %T", ct)
	}
	if len(ft.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(ft.Params))
	}
	if ft.Params[0].Name != "x" {
		t.Fatalf("param name: got %q, want %q", ft.Params[0].Name, "x")
	}
	if ft.Params[0].Type != PrimU32 {
		t.Fatalf("param type: got %v, want PrimU32", ft.Params[0].Type)
	}
	if len(ft.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(ft.Results))
	}
	if ft.Results[0].Name != "" {
		t.Fatalf("result name: got %q, want empty", ft.Results[0].Name)
	}
	if ft.Results[0].Type != PrimString {
		t.Fatalf("result type: got %v, want PrimString", ft.Results[0].Type)
	}
}

// TestComponentFuncType_NoResults tests a func type with no results (named list with 0 entries).
func TestComponentFuncType_NoResults(t *testing.T) {
	data := []byte{
		0x40,       // sync func type
		0x00,       // 0 params
		0x01, 0x00, // named result list with 0 entries = no results
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	ct, err := readComponentTypeDef(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ft, ok := ct.(*ComponentFuncType)
	if !ok {
		t.Fatalf("expected *ComponentFuncType, got %T", ct)
	}
	if len(ft.Params) != 0 {
		t.Fatalf("expected 0 params, got %d", len(ft.Params))
	}
	if len(ft.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(ft.Results))
	}
}

// TestComponentFuncType_InvalidResultByte tests that invalid result encoding bytes are rejected.
func TestComponentFuncType_InvalidResultByte(t *testing.T) {
	data := []byte{
		0x40,       // sync func type
		0x00,       // 0 params
		0x01, 0x01, // named result list with invalid inner byte (must be 0x00)
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	_, err := readComponentTypeDef(r)
	if err == nil {
		t.Fatal("expected error for invalid result encoding")
	}
}

// TestResourceType_NoDtor tests a resource type with i32 rep and no destructor.
func TestResourceType_NoDtor(t *testing.T) {
	data := []byte{
		0x3f, // resource type
		0x7f, // i32 rep
		0x00, // no dtor
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	ct, err := readComponentTypeDef(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rt, ok := ct.(*ResourceType)
	if !ok {
		t.Fatalf("expected *ResourceType, got %T", ct)
	}
	if rt.Rep != ValTypeI32 {
		t.Fatalf("rep: got %v, want ValTypeI32", rt.Rep)
	}
	if rt.Dtor.Valid {
		t.Fatal("expected no destructor")
	}
}

// TestResourceType_WithDtor tests a resource type with i64 rep and destructor.
func TestResourceType_WithDtor(t *testing.T) {
	data := []byte{
		0x3f, // resource type
		0x7e, // i64 rep
		0x01, // dtor present
		0x05, // dtor func index 5
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	ct, err := readComponentTypeDef(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rt, ok := ct.(*ResourceType)
	if !ok {
		t.Fatalf("expected *ResourceType, got %T", ct)
	}
	if rt.Rep != ValTypeI64 {
		t.Fatalf("rep: got %v, want ValTypeI64", rt.Rep)
	}
	if !rt.Dtor.Valid {
		t.Fatal("expected destructor to be present")
	}
	if rt.Dtor.Value != 5 {
		t.Fatalf("dtor index: got %d, want 5", rt.Dtor.Value)
	}
}

// TestReadComponentType_DefinedType tests that readComponentType dispatches to defined types.
func TestReadComponentType_DefinedType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want func(ComponentTypeDef) bool
	}{
		{
			name: "record",
			data: []byte{0x72, 0x01, 0x01, 'x', 0x7f}, // record with 1 field "x" bool
			want: func(ct ComponentTypeDef) bool {
				e, ok := ct.(*ComponentDefinedTypeEntry)
				if !ok {
					return false
				}
				_, ok = e.Type.(*RecordType)
				return ok
			},
		},
		{
			name: "list",
			data: []byte{0x70, 0x73}, // list<string>
			want: func(ct ComponentTypeDef) bool {
				e, ok := ct.(*ComponentDefinedTypeEntry)
				if !ok {
					return false
				}
				_, ok = e.Type.(*ListType)
				return ok
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader(tt.data), 0)
			ct, err := readComponentTypeDef(r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.want(ct) {
				t.Fatalf("type check failed for %T", ct)
			}
		})
	}
}
