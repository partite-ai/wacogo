package canon

import (
	"context"
	"errors"
	"math"
	"runtime"
	"testing"
)

// typeFunc adapts a visit function into a Type.
type typeFunc func(TypeVisitor)

func (f typeFunc) Accept(v TypeVisitor) { f(v) }

var (
	tU8     = typeFunc(func(v TypeVisitor) { v.VisitU8() })
	tU64    = typeFunc(func(v TypeVisitor) { v.VisitU64() })
	tString = typeFunc(func(v TypeVisitor) { v.VisitString() })
)

func tList(elem Type) Type { return typeFunc(func(v TypeVisitor) { v.VisitList(elem) }) }

func tRecord(fields ...Type) Type {
	rf := make([]RecordField, len(fields))
	for i, f := range fields {
		rf[i] = RecordField{Name: "f", Type: f}
	}
	return typeFunc(func(v TypeVisitor) { v.VisitRecord(rf) })
}

func tOption(inner Type) Type { return typeFunc(func(v TypeVisitor) { v.VisitOption(inner) }) }

func tResult(ok, err Type) Type { return typeFunc(func(v TypeVisitor) { v.VisitResult(ok, err) }) }

func tVariant(payloads ...Type) Type {
	cases := make([]VariantCase, len(payloads))
	for i, p := range payloads {
		cases[i] = VariantCase{Name: "c", Payload: p}
	}
	return typeFunc(func(v TypeVisitor) { v.VisitVariant(cases) })
}

// TestListLift_ChargesGoFootprint lifts a large list of each element
// shape out of zeroed memory and checks the budget charge tracks the Go
// heap bytes the lift actually allocates: never below it, and not wildly
// above it.
func TestListLift_ChargesGoFootprint(t *testing.T) {
	// Fits the largest element below in setupMem's single page.
	const n = 4000
	nest := func(depth int) Type {
		t := Type(tU8)
		for range depth {
			t = tRecord(t)
		}
		return t
	}
	for _, tc := range []struct {
		name string
		elem Type
	}{
		{"u8", tU8},
		{"u64", tU64},
		{"empty strings", tString},
		{"empty lists", tList(tU8)},
		{"record{u8}", tRecord(tU8)},
		{"record{8 x u8}", tRecord(tU8, tU8, tU8, tU8, tU8, tU8, tU8, tU8)},
		{"record depth 8", nest(8)},
		{"option<u64>", tOption(tU64)},
		{"result<u64, string>", tResult(tU64, tString)},
		{"variant<record{u8}, u8>", tVariant(tRecord(tU8), tU8)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := setupMem(t, 1<<16)
			v := &flatToValVisitor{}
			v.VisitList(tc.elem)
			gcc := &gocallContext{
				callee:    &transferSide{Memory: mem, StringEncoding: EncUTF8},
				registers: []uint64{0, n},
			}
			step := v.out[0]

			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			got, err := step(context.Background(), gcc, 0)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatalf("lift: %v", err)
			}
			runtime.KeepAlive(got)

			alloc := after.TotalAlloc - before.TotalAlloc
			charged := gcc.budget.used
			if gcc.budget.limit == 0 {
				// Unlimited budgets do not count; recompute what a
				// limited one would have charged.
				charged = listElemCharge(n, valFootprint(tc.elem))
			}
			t.Logf("allocated %d, charged %d (%.2fx)", alloc, charged, float64(charged)/float64(alloc))
			// Slack covers fixed per-lift allocations and size-class
			// rounding.
			const slack = 8 << 10
			if alloc > charged+charged/20+slack {
				t.Errorf("allocated %d bytes but charged only %d", alloc, charged)
			}
			if charged > 3*alloc+slack {
				t.Errorf("charged %d bytes for %d allocated", charged, alloc)
			}
		})
	}
}

// TestListLift_FootprintExceedsLimit checks that a list whose canonical
// size fits the limit is still rejected, before allocating its elements,
// when its Go footprint does not.
func TestListLift_FootprintExceedsLimit(t *testing.T) {
	const n = 1000
	mem := setupMem(t, 1<<16)
	elem := tRecord(tRecord(tRecord(tU8)))
	v := &flatToValVisitor{}
	v.VisitList(elem)
	charge := listElemCharge(n, valFootprint(elem))
	for _, tc := range []struct {
		name  string
		limit uint64
		ok    bool
	}{
		{"canonical size", n, false},
		{"one under", charge - 1, false},
		{"at footprint", charge, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gcc := &gocallContext{
				callee:    &transferSide{Memory: mem, StringEncoding: EncUTF8},
				registers: []uint64{0, n},
				budget:    LiftBudget{limit: tc.limit},
			}
			_, err := v.out[0](context.Background(), gcc, 0)
			if tc.ok {
				if err != nil {
					t.Fatalf("lift: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrLiftLimitExceeded) {
				t.Fatalf("err = %v, want ErrLiftLimitExceeded", err)
			}
		})
	}
}

func TestListElemCharge_Saturates(t *testing.T) {
	if got := listElemCharge(math.MaxUint32, math.MaxUint64-1); got != math.MaxUint64 {
		t.Fatalf("listElemCharge = %d, want saturation", got)
	}
}
