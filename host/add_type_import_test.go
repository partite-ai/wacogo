package host_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
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
