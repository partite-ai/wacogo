package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/acl"
)

type myAcl struct{}

func (myAcl) Grant(ctx context.Context, p acl.Perms) (acl.Perms, error) {
	return p | acl.PermsRead, nil // always grant read
}

func TestAcl_FlagsRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := acl.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myAcl{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (import "h" (instance $h
    (type $perms (flags "read" "write" "exec"))
    (export "perms" (type (eq $perms)))
    (export "grant" (func (param "p" $perms) (result $perms)))
  ))
  (alias export $h "grant" (func $g))
  (export "grant" (func $g))
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

	fn := inst.ExportedFunc("grant")

	// Grant Write → should get Write|Read
	in := wacogo.NewValFlags([]string{"read", "write", "exec"}, "write")
	got, err := fn.Call(ctx, in)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	f := got[0].(*wacogo.ValFlags)
	if !f.Has("read") {
		t.Errorf("expected read bit set; got %+v", f.Bits())
	}
	if !f.Has("write") {
		t.Errorf("expected write bit set; got %+v", f.Bits())
	}
	if f.Has("exec") {
		t.Errorf("did not expect exec bit set; got %+v", f.Bits())
	}
}
