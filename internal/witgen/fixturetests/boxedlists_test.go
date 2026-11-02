package fixturetests_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"unsafe"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/boxedlists"
)

const boxedListLen = 2048

// boxedListsGuestWAT returns boxedListLen-element lists whose elements
// each box a case struct: items are all text(""), tagged entries all
// carry ok("").
var boxedListsGuestWAT = fmt.Sprintf(`(component
  (core module $m
    (memory (export "mem") 2)
    (func (export "items") (result i32) (local $i i32)
      (block $done (loop $l
        (br_if $done (i32.ge_u (local.get $i) (i32.const %[1]d)))
        (i32.store8 (i32.add (i32.const 0x100) (i32.mul (local.get $i) (i32.const 12))) (i32.const 1))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $l)))
      (i32.store (i32.const 8) (i32.const 0x100))
      (i32.store (i32.const 12) (i32.const %[1]d))
      (i32.const 8))
    (func (export "tagged") (result i32)
      (i32.store (i32.const 16) (i32.const 0x10000))
      (i32.store (i32.const 20) (i32.const %[1]d))
      (i32.const 16)))
  (core instance $i (instantiate $m))
  (type $item' (variant (case "num" u32) (case "text" string) (case "empty")))
  (export $item "item" (type $item'))
  (type $tagged' (record (field "id" u32) (field "outcome" (result string (error u32)))))
  (export $tagged "tagged" (type $tagged'))
  (func (export "items") (result (list $item))
    (canon lift (core func $i "items") (memory $i "mem")))
  (func (export "tagged-list") (result (list $tagged))
    (canon lift (core func $i "tagged") (memory $i "mem")))
)`, boxedListLen)

// TestBoxedLists_LiftChargesGoFootprint checks that lifting a list whose
// elements box a case struct charges the boxed structs too: the limit
// sits exactly at the Go footprint, and the bytes actually allocated stay
// within it.
func TestBoxedLists_LiftChargesGoFootprint(t *testing.T) {
	for _, tc := range []struct {
		name     string
		elemCost uintptr
		call     func(context.Context, boxedlists.Boxedlists) (int, error)
	}{
		{
			name:     "variant",
			elemCost: unsafe.Sizeof(boxedlists.Item(nil)) + max(unsafe.Sizeof(boxedlists.ItemNum{}), unsafe.Sizeof(boxedlists.ItemText{})),
			call: func(ctx context.Context, w boxedlists.Boxedlists) (int, error) {
				got, err := w.Items(ctx)
				return len(got), err
			},
		},
		{
			name:     "record with result",
			elemCost: unsafe.Sizeof(boxedlists.Tagged{}) + max(unsafe.Sizeof(boxedlists.ResultStringU32Ok{}), unsafe.Sizeof(boxedlists.ResultStringU32Err{})),
			call: func(ctx context.Context, w boxedlists.Boxedlists) (int, error) {
				got, err := w.TaggedList(ctx)
				return len(got), err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			charge := uint64(boxedListLen) * uint64(tc.elemCost)

			ctx := context.Background()
			w := newBoxedListsWrapper(t, charge-1)
			if _, err := tc.call(ctx, w); !errors.Is(err, wacogo.ErrLiftLimitExceeded) {
				t.Fatalf("one byte under the footprint: err = %v, want ErrLiftLimitExceeded", err)
			}

			w = newBoxedListsWrapper(t, charge)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			n, err := tc.call(ctx, w)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatalf("at the footprint: %v", err)
			}
			if n != boxedListLen {
				t.Fatalf("lifted %d elements, want %d", n, boxedListLen)
			}
			// Slack covers the call's fixed allocations.
			const slack = 16 << 10
			if alloc := after.TotalAlloc - before.TotalAlloc; alloc > charge+slack {
				t.Fatalf("allocated %d bytes but charged %d", alloc, charge)
			}
		})
	}
}

func newBoxedListsWrapper(t *testing.T, limit uint64) boxedlists.Boxedlists {
	t.Helper()
	ctx := context.Background()
	e := wacogo.NewEngine(ctx, wacogo.WithMaxLiftBytes(limit))
	t.Cleanup(func() { _ = e.Close(ctx) })

	comp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, boxedListsGuestWAT)))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	callee, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	t.Cleanup(func() { _ = callee.Close(ctx) })

	hc, err := e.NewHostBuilder("caller").Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = hc.Close(ctx) })
	caller, err := hc.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate caller: %v", err)
	}
	t.Cleanup(func() { _ = caller.Close(ctx) })
	return boxedlists.WrapInstance(caller, callee)
}
