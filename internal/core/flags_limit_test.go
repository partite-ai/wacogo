package core

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestComponent_RejectsFlagsOver32Labels — the canonical-ABI MVP caps
// `flags` label count at 32. A 33-label flags type must be rejected;
// the specific surface (load, instantiate, or call time) is an
// implementation choice, but the round-trip must fail with a trap that
// names `flags` and the 32-label cap.
func TestComponent_RejectsFlagsOver32Labels(t *testing.T) {
	names := make([]string, 33)
	for i := range names {
		names[i] = fmt.Sprintf(`"f%d"`, i)
	}
	flagList := strings.Join(names, " ")

	wat := fmt.Sprintf(`(component
  (core module $m
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 0)
    (func (export "identity") (param i32) (result i32) local.get 0))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $realloc))
  (alias core export $i "identity" (core func $id))
  (type $fl (flags %s))
  (type $ft (func (param "f" $fl) (result $fl)))
  (func $lifted (type $ft) (canon lift (core func $id) (memory $mem) (realloc $realloc)))
  (export "identity" (func $lifted)))`, flagList)

	bin := buildComponentBytes(t, wat)

	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	comp, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		requireMentionsFlagsAnd32(t, "load", err)
		return
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Logf("rejected at instantiate: %v", err)
		return
	}
	defer inst.Close(ctx)

	fn := inst.ExportedFunc("identity")
	if fn == nil {
		t.Fatal("export 'identity' missing")
	}

	labels := make([]string, 33)
	for i := range labels {
		labels[i] = fmt.Sprintf("f%d", i)
	}

	defer func() {
		// Func.Call recovers *canon.Trap into an error; a raw panic is a bug.
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic from Func.Call: %v", r)
		}
	}()

	_, err = fn.Call(ctx, NewValFlags(labels))
	if err == nil {
		t.Fatal("expected call to fail with a >32 flag label trap")
	}
	requireMentionsFlagsAnd32(t, "call", err)
}

func requireMentionsFlagsAnd32(t *testing.T, stage string, err error) {
	t.Helper()
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "flag") {
		t.Errorf("%s error didn't mention flags: %v", stage, err)
	}
	if !strings.Contains(msg, "32") {
		t.Errorf("%s error didn't mention the 32-label cap: %v", stage, err)
	}
}
