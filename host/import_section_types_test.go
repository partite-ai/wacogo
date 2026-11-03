package host_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/core"
)

// Consecutive type imports share one import section. An eq-bound import
// must not end processing of that section: every later import still owns
// a type index, or each following index resolves one slot too low.
func TestEqTypeImportsKeepLaterImportsInSection(t *testing.T) {
	ctx := t.Context()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	bin := watToBinary(t, `(component
  (type $id string)
  (type $pos (record (field "line" u32)))
  (import "import-type-id" (type $id-import (eq $id)))
  (import "import-type-pos" (type $pos-import (eq $pos)))
  (type $positions (list $pos-import))
  (export "positions" (type $positions))
)`)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	want := core.TypeList{Elem: core.TypeRecord{Fields: []core.FieldType{{Name: "line", Type: core.TypeU32{}}}}}
	if got := inst.ExportedType("positions"); !reflect.DeepEqual(got, want) {
		t.Errorf("ExportedType(positions) = %#v, want %#v", got, want)
	}
}
