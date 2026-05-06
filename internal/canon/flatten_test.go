package canon

import (
	"reflect"
	"testing"
)

func TestFlattenTypeOf(t *testing.T) {
	tests := []struct {
		name string
		typ  Type
		want []coreValueType
	}{
		{
			name: "primitive s32 → [i32]",
			typ:  testTypeS32{},
			want: []coreValueType{coreI32},
		},
		{
			name: "string → [i32, i32]",
			typ:  testTypeString{},
			want: []coreValueType{coreI32, coreI32},
		},
		{
			name: "variant{s32, f64} → [i32, i64]",
			// variant discriminant = i32, payload = join(i32, f64) = i64
			typ: testTypeVariant{cases: []VariantCase{
				{Name: "a", Payload: testTypeS32{}},
				{Name: "b", Payload: testTypeF64{}},
			}},
			want: []coreValueType{coreI32, coreI64},
		},
		{
			name: "bool → [i32]",
			typ:  testTypeBool{},
			want: []coreValueType{coreI32},
		},
		{
			name: "u64 → [i64]",
			typ:  testTypeU64{},
			want: []coreValueType{coreI64},
		},
		{
			name: "f32 → [f32]",
			typ:  testTypeF32{},
			want: []coreValueType{coreF32},
		},
		{
			name: "f64 → [f64]",
			typ:  testTypeF64{},
			want: []coreValueType{coreF64},
		},
		{
			name: "record{s32, f64} → [i32, f64]",
			typ: testTypeRecord{fields: []RecordField{
				{Name: "x", Type: testTypeS32{}},
				{Name: "y", Type: testTypeF64{}},
			}},
			want: []coreValueType{coreI32, coreF64},
		},
		{
			name: "list → [i32, i32]",
			typ:  testTypeList{elem: testTypeS32{}},
			want: []coreValueType{coreI32, coreI32},
		},
		{
			name: "option(f64) → [i32, f64]",
			typ:  testTypeOption{inner: testTypeF64{}},
			want: []coreValueType{coreI32, coreF64},
		},
		{
			name: "result(s32, f64) → [i32, i64]",
			// disc=i32, payload=join(i32, f64)=i64
			typ:  testTypeResult{ok: testTypeS32{}, err: testTypeF64{}},
			want: []coreValueType{coreI32, coreI64},
		},
		{
			name: "variant{f32, i32} → [i32, i32] (f32 join i32 = i32)",
			typ: testTypeVariant{cases: []VariantCase{
				{Name: "a", Payload: testTypeF32{}},
				{Name: "b", Payload: testTypeS32{}},
			}},
			want: []coreValueType{coreI32, coreI32},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenTypeOf(tt.typ)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("flattenTypeOf(%T) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestJoinCoreValueType(t *testing.T) {
	tests := []struct {
		a, b coreValueType
		want coreValueType
	}{
		{coreI32, coreI32, coreI32},
		{coreI64, coreI64, coreI64},
		{coreF32, coreF32, coreF32},
		{coreF64, coreF64, coreF64},
		{coreI32, coreI64, coreI64},
		{coreI64, coreI32, coreI64},
		{coreI32, coreF32, coreI32},
		{coreF32, coreI32, coreI32},
		{coreI32, coreF64, coreI64},
		{coreF64, coreI32, coreI64},
		{coreI64, coreF32, coreI64},
		{coreF32, coreI64, coreI64},
		{coreI64, coreF64, coreI64},
		{coreF64, coreI64, coreI64},
		{coreF32, coreF64, coreI64},
		{coreF64, coreF32, coreI64},
	}
	for _, tt := range tests {
		got := joinCoreValueType(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("joinCoreValueType(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// --- minimal stub types implementing canon.Type for tests ---

type testTypeBool struct{}
type testTypeS32 struct{}
type testTypeU64 struct{}
type testTypeF32 struct{}
type testTypeF64 struct{}
type testTypeString struct{}
type testTypeList struct{ elem Type }
type testTypeRecord struct{ fields []RecordField }
type testTypeVariant struct{ cases []VariantCase }
type testTypeOption struct{ inner Type }
type testTypeResult struct{ ok, err Type }

func (testTypeBool) Accept(v TypeVisitor)   { v.VisitBool() }
func (testTypeS32) Accept(v TypeVisitor)    { v.VisitS32() }
func (testTypeU64) Accept(v TypeVisitor)    { v.VisitU64() }
func (testTypeF32) Accept(v TypeVisitor)    { v.VisitF32() }
func (testTypeF64) Accept(v TypeVisitor)    { v.VisitF64() }
func (testTypeString) Accept(v TypeVisitor) { v.VisitString() }
func (t testTypeList) Accept(v TypeVisitor) { v.VisitList(t.elem) }
func (t testTypeRecord) Accept(v TypeVisitor) {
	v.VisitRecord(t.fields)
}
func (t testTypeVariant) Accept(v TypeVisitor) { v.VisitVariant(t.cases) }
func (t testTypeOption) Accept(v TypeVisitor)  { v.VisitOption(t.inner) }
func (t testTypeResult) Accept(v TypeVisitor)  { v.VisitResult(t.ok, t.err) }
