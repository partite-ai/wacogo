package fixturetests_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/filesystem"
	streams "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/streams"
)

// TestFilesystem_ErrorPaths exercises the failure modes of the cross-package
// binding and dispatch machinery. Each subtest verifies that an invalid
// usage produces a non-silent failure — an error or a panic — and never
// silently succeeds.
func TestFilesystem_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	streamsFac, err := streams.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer streamsFac.Close(ctx)

	streamsInst, err := streamsFac.NewInstance(ctx, &myStreams{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer streamsInst.Close(ctx)

	fsFac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fsFac.Close(ctx)

	// Build a valid filesystem instance for use in WrongInstanceKind subtest.
	goodFsInst, err := fsFac.NewInstance(ctx, &myFilesystem{}, &filesystem.Deps{Streams: streamsInst.Core()})
	if err != nil {
		t.Fatal(err)
	}
	defer goodFsInst.Close(ctx)

	t.Run("WrongInstanceKind", func(t *testing.T) {
		// Build a host instance that does NOT export "handle" — it exports
		// an unrelated resource. fsFac.NewInstance forwards the dep via
		// host.WithResourceFrom(handleRef, dep, "handle"), so the lender-
		// validation path must reject this with a non-nil error.
		b := e.NewHostBuilder("wrong_kind")
		_ = b.AddResource("not_handle", nil)
		wrongComp, err := b.Build(ctx)
		if err != nil {
			t.Fatalf("Build wrong-kind: %v", err)
		}
		defer wrongComp.Close(ctx)
		wrongInst, err := wrongComp.Instantiate(ctx)
		if err != nil {
			t.Fatalf("Instantiate wrong-kind: %v", err)
		}
		defer wrongInst.Close(ctx)

		_, err = fsFac.NewInstance(ctx, &myFilesystem{}, &filesystem.Deps{Streams: wrongInst.Core()})
		if err == nil {
			t.Fatal("expected error when dep does not export 'handle'; got nil")
		}
		t.Logf("got error (expected): %v", err)
	})

	t.Run("NilSourceInstance", func(t *testing.T) {
		// Pass nil where a *Deps is expected. filesystem.NewInstance panics
		// on a nil *Deps when the interface has imports.
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic when passing nil deps, got nil")
			}
			msg := fmt.Sprint(r)
			if !strings.Contains(msg, "deps is nil") {
				t.Errorf("panic message should contain \"deps is nil\"; got: %v", msg)
			}
			t.Logf("got panic (expected): %v", r)
		}()
		_, _ = fsFac.NewInstance(ctx, &myFilesystem{}, nil)
		t.Fatal("expected panic, NewInstance returned")
	})

	t.Run("ClosedSourceInstance", func(t *testing.T) {
		// Create a fresh streams instance, close it, then pass it as the
		// streams dep to filesystem.NewInstance. Whether the error is caught at
		// bind time or surfaces later (e.g., from wazero on first use) depends
		// on implementation details — the important invariant is that the path
		// does NOT silently succeed all the way through a subsequent call.

		fresh, err := streamsFac.NewInstance(ctx, &myStreams{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		fresh.Close(ctx)

		var bindErr error
		var fsInst *host.ComponentInstance
		fs := &myFilesystem{streamsInst: fresh}
		fsInst, bindErr = fsFac.NewInstance(ctx, fs, &filesystem.Deps{Streams: fresh.Core()})
		if bindErr != nil {
			// Error surfaced at bind time — ideal behaviour.
			t.Logf("got error at NewInstance (expected): %v", bindErr)
			return
		}
		fs.fsInst = fsInst
		defer fsInst.Close(ctx)

		// If NewInstance succeeded, the error must surface at call time.
		t.Log("NewInstance succeeded with closed source; verifying call fails")
		var callGot string
		func() {
			defer func() {
				if r := recover(); r != nil {
					callGot = "panic"
					t.Logf("got panic on call after closed source: %v", r)
				}
			}()
			fsWrap := filesystem.WrapInstance(fsInst, fsInst.Core())
			h, openErr := fsWrap.Open(ctx)
			if openErr != nil {
				callGot = "error"
				t.Logf("got error on Open after closed source: %v", openErr)
				return
			}
			if _, readErr := h.Read(ctx); readErr != nil {
				callGot = "error"
				t.Logf("got error on Read after closed source: %v", readErr)
				return
			}
			callGot = "no-error"
		}()
		if callGot == "no-error" {
			t.Errorf("ClosedSourceInstance: expected error or panic, got silent success")
		}
	})

	t.Run("UseAfterClose", func(t *testing.T) {
		// Create a fresh streams instance, bind a filesystem instance to it,
		// obtain a Handle via Open(), then close the underlying streams instance.
		// A subsequent call to Read() on the Handle must not silently succeed.

		fresh, err := streamsFac.NewInstance(ctx, &myStreams{}, nil)
		if err != nil {
			t.Fatal(err)
		}

		fs := &myFilesystem{streamsInst: fresh}
		fsInst, err := fsFac.NewInstance(ctx, fs, &filesystem.Deps{Streams: fresh.Core()})
		if err != nil {
			t.Fatal(err)
		}
		fs.fsInst = fsInst
		defer fsInst.Close(ctx)

		fsWrap := filesystem.WrapInstance(fsInst, fsInst.Core())
		h, err := fsWrap.Open(ctx)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}

		// Close the backing streams instance. Any subsequent call that reaches
		// into wazero's closed module must fail.
		fresh.Close(ctx)

		var got string
		func() {
			defer func() {
				if r := recover(); r != nil {
					got = "panic"
					t.Logf("Read() after close panicked: %v", r)
				}
			}()
			if _, readErr := h.Read(ctx); readErr != nil {
				got = "error"
				t.Logf("Read() after close errored: %v", readErr)
				return
			}
			got = "no-error"
		}()
		if got == "no-error" {
			t.Errorf("UseAfterClose: Read() after close silently succeeded; expected panic or error")
		}
		t.Logf("Read() after close: %s", got)
	})

	// WrongSourceHandle: binding a handle from instA and passing it to a
	// filesystem bound to instB. Add this subtest once the implementer-side
	// bound-instance check is in place. Without it the canon table would
	// reject a rep from the wrong instance's table at dispatch time, but
	// the exact error surface (panic vs. error) is not yet defined.
}
