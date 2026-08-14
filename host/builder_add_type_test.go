package host_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

func TestAddTypeExportsSatisfyConsumerInstanceSubtype(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("typed-host")
	b.AddType("problem", host.Variant{Cases: []host.Case{
		{Name: "denied"},
		{Name: "detail", Payload: host.String},
	}})
	b.AddType("", host.List{Elem: host.U32})
	nested := b.AddNestedInstance("nested")
	nested.AddType("problem", host.Record{Fields: []host.Field{
		{Name: "code", Type: host.U32},
	}})

	hostComp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("host Build: %v", err)
	}
	defer hostComp.Close(ctx)
	hostInst, err := hostComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("host Instantiate: %v", err)
	}
	defer hostInst.Close(ctx)

	tests := []struct {
		name       string
		problem    string
		nestedCode string
		wantErr    bool
	}{
		{
			name:       "matching root and nested types",
			problem:    `(variant (case "denied") (case "detail" string))`,
			nestedCode: "u32",
		},
		{
			name:       "mismatched root type",
			problem:    `(variant (case "denied") (case "detail" u32))`,
			nestedCode: "u32",
			wantErr:    true,
		},
		{
			name:       "mismatched nested type",
			problem:    `(variant (case "denied") (case "detail" string))`,
			nestedCode: "s32",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guestWAT := `(component
  (import "host" (instance
    (type $problem ` + tt.problem + `)
    (export "problem" (type (eq $problem)))
    (export "nested" (instance
      (type $nested-problem (record (field "code" ` + tt.nestedCode + `)))
      (export "problem" (type (eq $nested-problem)))
    ))
  ))
)`
			guestBin := watToBinary(t, guestWAT)
			guestComp, err := e.LoadComponent(ctx, bytes.NewReader(guestBin))
			if err != nil {
				t.Fatalf("guest LoadComponent: %v", err)
			}
			guestInst, err := guestComp.Instantiate(ctx,
				wacogo.WithInstanceImport("host", hostInst.Core()))
			if tt.wantErr {
				if err == nil {
					_ = guestInst.Close(ctx)
					t.Fatal("guest Instantiate succeeded with a mismatched named type")
				}
				return
			}
			if err != nil {
				t.Fatalf("guest Instantiate: %v", err)
			}
			defer guestInst.Close(ctx)
		})
	}
}

// TestAddTypeExportsResolveAtRuntime covers the other half of an AddType
// export: a consumer that aliases the type out of the imported instance
// must resolve it to a concrete Type, at the root and inside a nested
// instance.
func TestAddTypeExportsResolveAtRuntime(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("typed-host")
	b.AddType("problem", host.Variant{Cases: []host.Case{
		{Name: "denied"},
		{Name: "detail", Payload: host.String},
	}})
	nested := b.AddNestedInstance("nested")
	nested.AddType("code", host.Record{Fields: []host.Field{
		{Name: "code", Type: host.U32},
	}})

	hostComp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("host Build: %v", err)
	}
	defer hostComp.Close(ctx)
	hostInst, err := hostComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("host Instantiate: %v", err)
	}
	defer hostInst.Close(ctx)

	wantProblem := core.TypeVariant{Cases: []core.CaseType{
		{Name: "denied"},
		{Name: "detail", Payload: core.TypeString{}},
	}}
	if got := hostInst.Core().ExportedType("problem"); !reflect.DeepEqual(got, wantProblem) {
		t.Errorf("root ExportedType(problem) = %#v, want %#v", got, wantProblem)
	}

	sub := hostInst.Core().ExportedInstance("nested")
	if sub == nil {
		t.Fatal("nested instance is not exported")
	}
	wantCode := core.TypeRecord{Fields: []core.FieldType{{Name: "code", Type: core.TypeU32{}}}}
	if got := sub.ExportedType("code"); !reflect.DeepEqual(got, wantCode) {
		t.Errorf("nested ExportedType(code) = %#v, want %#v", got, wantCode)
	}

	// A guest aliasing both type exports must instantiate cleanly.
	guestWAT := `(component
  (import "host" (instance $h
    (type $problem (variant (case "denied") (case "detail" string)))
    (export "problem" (type (eq $problem)))
    (export "nested" (instance
      (type $code (record (field "code" u32)))
      (export "code" (type (eq $code)))
    ))
  ))
  (alias export $h "problem" (type $p))
  (alias export $h "nested" (instance $n))
  (alias export $n "code" (type $c))
  (export "reexported-problem" (type $p))
  (export "reexported-code" (type $c))
)`
	guestComp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatalf("guest LoadComponent: %v", err)
	}
	guestInst, err := guestComp.Instantiate(ctx,
		wacogo.WithInstanceImport("host", hostInst.Core()))
	if err != nil {
		t.Fatalf("guest Instantiate: %v", err)
	}
	defer guestInst.Close(ctx)
}
