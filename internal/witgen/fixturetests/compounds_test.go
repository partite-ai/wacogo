package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/compounds"
)

// myCounter is a minimal Counter impl: holds a u32 returned by Get().
type myCounter struct{ v uint32 }

func (c *myCounter) Get(ctx context.Context) (uint32, error) { return c.v, nil }

// myCompounds implements the host-side Compounds interface. NewCounters
// returns a fresh slice of *CounterHandle, one per starting value, exercising
// the list<own<R>> result helpers (fromGoMemListCounter). BoxCounter wraps
// the incoming handle in a RecordWithCounter, exercising the
// record { c: own<R> } result helpers (fromGoFlatRecordWithCounter).
type myCompounds struct{}

func (myCompounds) NewCounters(ctx context.Context, starts []uint32) ([]*compounds.CounterHandle, error) {
	out := make([]*compounds.CounterHandle, len(starts))
	for i, s := range starts {
		out[i] = compounds.NewCounterHandle(&myCounter{v: s})
	}
	return out, nil
}

func (myCompounds) BoxCounter(ctx context.Context, c *compounds.CounterHandle) (compounds.RecordWithCounter, error) {
	return compounds.RecordWithCounter{C: c}, nil
}

func (myCompounds) NewCounter(ctx context.Context, start uint32) (*compounds.CounterHandle, error) {
	return compounds.NewCounterHandle(&myCounter{v: start}), nil
}

// TestCompounds_NewCounters drives the list<own<counter>> result path.
// A wasm client calls new-counters([1, 2, 3]) and reads back the get()
// value of each returned handle, verifying that the list-of-handles is
// constructed correctly through fromGoMemListCounter and that each handle
// is properly bound in the importer's canon table.
func TestCompounds_NewCounters(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := compounds.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myCompounds{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	// Importer WAT: imports the compounds interface, calls new-counters
	// with [1, 2, 3], then iterates the returned list and calls get() on
	// each handle. Returns the sum (1 + 2 + 3 = 6) so the test can assert
	// the round-trip succeeded.
	importerWAT := `(component
  (import "h" (instance $h
    (export "counter" (type $c (sub resource)))
    (export "[constructor]counter" (func (param "start" u32) (result (own $c))))
    (export "[method]counter.get" (func (param "self" (borrow $c)) (result u32)))
    (export "new-counters" (func (param "starts" (list u32)) (result (list (own $c)))))
  ))
  (alias export $h "counter" (type $c))
  (alias export $h "[method]counter.get" (func $get))
  (alias export $h "new-counters" (func $newcs))

  (core func $get-lowered (canon lower (func $get)))
  (core func $drop (canon resource.drop $c))

  (core module $mem-mod
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      ;; trivial realloc: ignore inputs and return a fixed bump address.
      ;; Sufficient for tests that only realloc once at a known offset.
      i32.const 1024
    )
  )
  (core instance $memi (instantiate $mem-mod))
  (alias core export $memi "memory" (core memory $mem))
  (alias core export $memi "realloc" (core func $rea))

  (core func $newcs-lowered (canon lower (func $newcs) (memory $mem) (realloc $rea)))

  (core module $m
    (import "" "newcs" (func $newcs (param i32 i32 i32)))
    (import "" "get"   (func $get (param i32) (result i32)))
    (import "" "drop"  (func $drop (param i32)))
    (import "" "memory" (memory 1))
    (import "" "realloc" (func $realloc (param i32 i32 i32 i32) (result i32)))

    ;; layout: input list u32 at addr 0 [1, 2, 3]
    ;;        result list ptr returned in retArea at 16 (ptr,len)
    (func (export "test") (result i32)
      (local $sum i32)
      (local $listPtr i32)
      (local $listLen i32)
      (local $i i32)
      (local $h i32)
      (local $v i32)

      ;; write input list [1, 2, 3] at offset 0
      i32.const 0  i32.const 1  i32.store
      i32.const 4  i32.const 2  i32.store
      i32.const 8  i32.const 3  i32.store

      ;; call newcs(ptr=0, len=3, retArea=16)
      i32.const 0   ;; ptr to list
      i32.const 3   ;; len
      i32.const 16  ;; out ptr
      call $newcs

      ;; read back result list (ptr,len) from retArea
      i32.const 16  i32.load   local.set $listPtr
      i32.const 20  i32.load   local.set $listLen

      ;; iterate and sum get(handle)
      i32.const 0  local.set $i
      i32.const 0  local.set $sum
      block $exit
        loop $iter
          local.get $i
          local.get $listLen
          i32.ge_u
          br_if $exit

          ;; load handle at listPtr + i*4
          local.get $listPtr
          local.get $i
          i32.const 4
          i32.mul
          i32.add
          i32.load
          local.set $h

          ;; call get(h) -> v
          local.get $h
          call $get
          local.set $v

          ;; sum += v
          local.get $sum
          local.get $v
          i32.add
          local.set $sum

          ;; drop the handle (each is own)
          local.get $h
          call $drop

          ;; i++
          local.get $i  i32.const 1  i32.add  local.set $i
          br $iter
        end
      end
      local.get $sum
    )
  )
  (core instance $bridge
    (export "newcs" (func $newcs-lowered))
    (export "get" (func $get-lowered))
    (export "drop" (func $drop))
    (export "memory" (memory $mem))
    (export "realloc" (func $rea))
  )
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "test" (core func $t))
  (func (export "test") (result u32) (canon lift (core func $t)))
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

	fn := inst.ExportedFunc("test")
	if fn == nil {
		t.Fatal("want exported test")
	}
	got, err := fn.Call(ctx)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if u := uint32(got[0].(wacogo.ValU32)); u != 6 {
		t.Fatalf("sum of get(h) for new-counters([1,2,3]): got %d, want 6", u)
	}
}

// TestCompounds_BoxCounter drives the record { c: own<counter> } path.
// A wasm client constructs a counter, calls box-counter on it, then reads
// the record's c field back as an own handle and calls get() on it. This
// exercises the trampoline ParamLift path for own<R> (the box-counter
// argument), the from-Go record-with-own path for the result struct's c
// field (fromGoFlatRecordWithCounter), and the to-Go record path on the
// wasm side via canon's lift.
func TestCompounds_BoxCounter(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := compounds.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myCompounds{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	// The record-with-counter return is mem-mode (one i32 field, but the
	// canonical-ABI return goes through realloc into linear memory). The
	// wasm client constructs a counter at start=42, calls box-counter, then
	// reads the c field handle back from memory and calls get(c).
	importerWAT := `(component
  (import "h" (instance $h
    (export "counter" (type $c (sub resource)))
    (export "[constructor]counter" (func (param "start" u32) (result (own $c))))
    (export "[method]counter.get" (func (param "self" (borrow $c)) (result u32)))
    (type $rwc (record (field "c" (own $c))))
    (export "record-with-counter" (type (eq $rwc)))
    (export "box-counter" (func (param "c" (own $c)) (result $rwc)))
  ))
  (alias export $h "counter" (type $c))
  (alias export $h "[constructor]counter" (func $new))
  (alias export $h "[method]counter.get" (func $get))
  (alias export $h "box-counter" (func $box))

  (core func $new-lowered (canon lower (func $new)))
  (core func $get-lowered (canon lower (func $get)))
  (core func $drop (canon resource.drop $c))

  (core module $mem-mod
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      i32.const 1024
    )
  )
  (core instance $memi (instantiate $mem-mod))
  (alias core export $memi "memory" (core memory $mem))
  (alias core export $memi "realloc" (core func $rea))

  (core func $box-lowered (canon lower (func $box) (memory $mem) (realloc $rea)))

  (core module $m
    (import "" "new"  (func $new (param i32) (result i32)))
    (import "" "get"  (func $get (param i32) (result i32)))
    (import "" "box"  (func $box (param i32) (result i32)))
    (import "" "drop" (func $drop (param i32)))

    (func (export "test") (result i32)
      (local $h i32)
      (local $boxed_h i32)
      (local $v i32)

      ;; new(42) -> $h
      i32.const 42
      call $new
      local.set $h

      ;; box($h) -> boxed_h (single-field record flattens to one i32)
      local.get $h
      call $box
      local.set $boxed_h

      ;; get(boxed_h) -> v
      local.get $boxed_h
      call $get
      local.set $v

      ;; drop the boxed handle
      local.get $boxed_h
      call $drop

      local.get $v
    )
  )
  (core instance $bridge
    (export "new" (func $new-lowered))
    (export "get" (func $get-lowered))
    (export "box" (func $box-lowered))
    (export "drop" (func $drop))
    (export "memory" (memory $mem))
  )
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "test" (core func $t))
  (func (export "test") (result u32) (canon lift (core func $t)))
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

	fn := inst.ExportedFunc("test")
	if fn == nil {
		t.Fatal("want exported test")
	}
	got, err := fn.Call(ctx)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if u := uint32(got[0].(wacogo.ValU32)); u != 42 {
		t.Fatalf("get(box-counter(new(42)).c): got %d, want 42", u)
	}
}

// TestCompounds_NewCountersHandles_GoRoundTrip exercises the Go-only
// construction path: it calls the impl's NewCounters and BoxCounter
// directly (no wasm), then confirms each returned *CounterHandle
// dispatches the Get() method correctly through the embedded Counter.
// The trampoline-side helpers (fromGoMemListCounter,
// fromGoFlatRecordWithCounter) are exercised in the wasm tests above.
func TestCompounds_NewCountersHandles_GoRoundTrip(t *testing.T) {
	ctx := context.Background()
	impl := myCompounds{}
	hs, err := impl.NewCounters(ctx, []uint32{10, 20, 30})
	if err != nil {
		t.Fatalf("NewCounters: %v", err)
	}
	if len(hs) != 3 {
		t.Fatalf("len(hs): got %d, want 3", len(hs))
	}
	wants := []uint32{10, 20, 30}
	for i, h := range hs {
		if h == nil {
			t.Fatalf("hs[%d] = nil", i)
		}
		got, err := h.Get(ctx)
		if err != nil {
			t.Fatalf("hs[%d].Get: %v", i, err)
		}
		if got != wants[i] {
			t.Errorf("hs[%d].Get(): got %d, want %d", i, got, wants[i])
		}
	}

	// BoxCounter: wraps the incoming handle in a RecordWithCounter; the
	// embedded handle's Get must round-trip through the record's C field.
	c := compounds.NewCounterHandle(&myCounter{v: 99})
	rec, err := impl.BoxCounter(ctx, c)
	if err != nil {
		t.Fatalf("BoxCounter: %v", err)
	}
	if rec.C == nil {
		t.Fatal("rec.C = nil, want the boxed handle")
	}
	got, err := rec.C.Get(ctx)
	if err != nil {
		t.Fatalf("rec.C.Get: %v", err)
	}
	if got != 99 {
		t.Errorf("rec.C.Get(): got %d, want 99", got)
	}
}
