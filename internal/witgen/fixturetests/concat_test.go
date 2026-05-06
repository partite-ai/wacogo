package fixturetests_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/concat"
)

type myConcat struct{}

func (myConcat) Join(ctx context.Context, parts []string, sep string) (string, error) {
	return strings.Join(parts, sep), nil
}

func TestConcat_ListStringRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := concat.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myConcat{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (type $ft (func (param "parts" (list string)) (param "sep" string) (result string)))
  (import "h" (instance $h
    (export "join" (func (type $ft)))
  ))
  (alias export $h "join" (func $j))
  (export "join" (func $j))
)`
	bin := watToBinary(t, importerWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx, wacogo.WithInstanceImport("h", hostInst.Core()))
	if err != nil {
		t.Fatalf("Instantiate importer: %v", err)
	}
	defer inst.Close(ctx)

	fn := inst.ExportedFunc("join")
	parts := wacogo.NewValListOf[wacogo.ValString]("a", "b", "c")
	sep := wacogo.ValString("-")
	got, err := fn.Call(ctx, parts, sep)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if s := string(got[0].(wacogo.ValString)); s != "a-b-c" {
		t.Fatalf("got %q, want %q", s, "a-b-c")
	}
}
