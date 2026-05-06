package host_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

func TestCrossComponent_ResourceTypeShared(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Lender: declares "stream" resource.
	lb := e.NewHostBuilder("lender")
	_ = lb.AddResource("stream", nil)
	lender, err := lb.Build(ctx)
	if err != nil {
		t.Fatalf("lender Build: %v", err)
	}
	defer lender.Close(ctx)
	lenderInst, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("lender Instantiate: %v", err)
	}
	defer lenderInst.Close(ctx)

	lenderTR, ok := lenderInst.Core().ExportedType("stream").(*core.TypeResource)
	if !ok {
		t.Fatalf("lender stream export: got %T, want *core.TypeResource", lenderInst.Core().ExportedType("stream"))
	}

	// Consumer: re-exports "stream" via AddResourceRef.
	cb := e.NewHostBuilder("consumer")
	streamRef := cb.AddResourceRef("stream")
	consumer, err := cb.Build(ctx)
	if err != nil {
		t.Fatalf("consumer Build: %v", err)
	}
	defer consumer.Close(ctx)

	consumerInst, err := consumer.Instantiate(ctx,
		host.WithResourceFrom(streamRef, lenderInst.Core(), "stream"))
	if err != nil {
		t.Fatalf("consumer Instantiate: %v", err)
	}
	defer consumerInst.Close(ctx)

	consumerTR, ok := consumerInst.Core().ExportedType("stream").(*core.TypeResource)
	if !ok {
		t.Fatalf("consumer stream export: got %T, want *core.TypeResource", consumerInst.Core().ExportedType("stream"))
	}
	if consumerTR != lenderTR {
		t.Fatalf("type identity not shared: lender=%p consumer=%p", lenderTR, consumerTR)
	}
}

func TestCrossComponent_MissingWithResourceFrom_Errors(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	cb := e.NewHostBuilder("consumer_missing")
	_ = cb.AddResourceRef("stream")
	consumer, err := cb.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer consumer.Close(ctx)

	if _, err := consumer.Instantiate(ctx); err == nil {
		t.Fatal("Instantiate without WithResourceFrom: want error, got nil")
	}
}

func TestCrossComponent_WrongLenderExport_Errors(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Lender exports "foo" as a resource.
	lb := e.NewHostBuilder("lender_x")
	_ = lb.AddResource("foo", nil)
	lender, err := lb.Build(ctx)
	if err != nil {
		t.Fatalf("lender Build: %v", err)
	}
	defer lender.Close(ctx)
	lenderInst, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("lender Instantiate: %v", err)
	}
	defer lenderInst.Close(ctx)

	// Consumer asks for "bar" — doesn't exist on lender.
	cb := e.NewHostBuilder("consumer_x")
	ref := cb.AddResourceRef("bar")
	consumer, err := cb.Build(ctx)
	if err != nil {
		t.Fatalf("consumer Build: %v", err)
	}
	defer consumer.Close(ctx)

	if _, err := consumer.Instantiate(ctx, host.WithResourceFrom(ref, lenderInst.Core(), "bar")); err == nil {
		t.Fatal("Instantiate with non-existent lender export: want error, got nil")
	}
}
