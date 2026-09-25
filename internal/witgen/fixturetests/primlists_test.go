package fixturetests_test

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/primlists"
)

// revImpl returns each list reversed, so element order - and therefore
// the offset arithmetic of the bulk copy - is checked as well as values.
type revImpl struct{}

func rev[T any](xs []T) ([]T, error) {
	out := slices.Clone(xs)
	slices.Reverse(out)
	return out, nil
}

func (revImpl) RevBool(_ context.Context, xs []bool) ([]bool, error)      { return rev(xs) }
func (revImpl) RevS8(_ context.Context, xs []int8) ([]int8, error)        { return rev(xs) }
func (revImpl) RevU8(_ context.Context, xs []uint8) ([]uint8, error)      { return rev(xs) }
func (revImpl) RevS16(_ context.Context, xs []int16) ([]int16, error)     { return rev(xs) }
func (revImpl) RevU16(_ context.Context, xs []uint16) ([]uint16, error)   { return rev(xs) }
func (revImpl) RevS32(_ context.Context, xs []int32) ([]int32, error)     { return rev(xs) }
func (revImpl) RevU32(_ context.Context, xs []uint32) ([]uint32, error)   { return rev(xs) }
func (revImpl) RevS64(_ context.Context, xs []int64) ([]int64, error)     { return rev(xs) }
func (revImpl) RevU64(_ context.Context, xs []uint64) ([]uint64, error)   { return rev(xs) }
func (revImpl) RevF32(_ context.Context, xs []float32) ([]float32, error) { return rev(xs) }
func (revImpl) RevF64(_ context.Context, xs []float64) ([]float64, error) { return rev(xs) }
func (revImpl) RevChar(_ context.Context, xs []rune) ([]rune, error)      { return rev(xs) }

// TestPrimlists_RoundTrip sends a list of every primitive type through
// the generated wrapper into a host implementation and back: the
// wrapper lowers the list into the callee and lifts the result, and the
// host side reads the argument and writes the result, all through the
// bulk list copy.
func TestPrimlists_RoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := primlists.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)
	callee, err := fac.NewInstance(ctx, revImpl{}, nil)
	if err != nil {
		t.Fatalf("NewInstance callee: %v", err)
	}
	defer callee.Close(ctx)
	caller, err := fac.NewInstance(ctx, revImpl{}, nil)
	if err != nil {
		t.Fatalf("NewInstance caller: %v", err)
	}
	defer caller.Close(ctx)
	w := primlists.WrapInstance(caller, callee.Core())

	check(t, "bool", w.RevBool, []bool{true, false, false, true, true})
	check(t, "s8", w.RevS8, []int8{math.MinInt8, -1, 0, 1, math.MaxInt8})
	check(t, "u8", w.RevU8, []uint8{0, 1, 0x7f, 0x80, 0xff})
	check(t, "s16", w.RevS16, []int16{math.MinInt16, -2, 0, 3, math.MaxInt16})
	check(t, "u16", w.RevU16, []uint16{0, 1, 0x1234, 0xffff})
	check(t, "s32", w.RevS32, []int32{math.MinInt32, -3, 0, 4, math.MaxInt32})
	check(t, "u32", w.RevU32, []uint32{0, 1, 0xdeadbeef, math.MaxUint32})
	check(t, "s64", w.RevS64, []int64{math.MinInt64, -5, 0, 6, math.MaxInt64})
	check(t, "u64", w.RevU64, []uint64{0, 1, 0x0123456789abcdef, math.MaxUint64})
	check(t, "f32", w.RevF32, []float32{0, float32(math.Copysign(0, -1)), 1.5, -math.MaxFloat32, float32(math.Inf(1))})
	check(t, "f64", w.RevF64, []float64{0, math.Copysign(0, -1), math.Pi, -math.MaxFloat64, math.Inf(-1)})
	check(t, "char", w.RevChar, []rune{'a', 'é', '€', 0x10FFFF, 0})

	// Empty lists, and a large one (bigger than the stub's first page).
	check(t, "u8 empty", w.RevU8, []uint8{})
	big := make([]uint64, 20000)
	for i := range big {
		big[i] = uint64(i) * 0x9e3779b97f4a7c15
	}
	check(t, "u64 large", w.RevU64, big)

	// NaN keeps its bits (no canonicalization, as for single values).
	nan := math.Float64frombits(0x7ff8_0000_0000_0001)
	got, err := w.RevF64(ctx, []float64{nan})
	if err != nil {
		t.Fatalf("f64 NaN: %v", err)
	}
	if b := math.Float64bits(got[0]); b != 0x7ff8_0000_0000_0001 {
		t.Errorf("f64 NaN bits = %#x", b)
	}
}

func check[T comparable](t *testing.T, name string, f func(context.Context, []T) ([]T, error), in []T) {
	t.Helper()
	got, err := f(context.Background(), in)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	want, _ := rev(in)
	if !slices.Equal(got, want) {
		t.Errorf("%s: got %v, want %v", name, got, want)
	}
}
