package canon

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// StringEncoding selects between the three canonical-ABI string encodings.
// Root uses this directly (via the canonicalOptions struct) — there is no
// parent-package enum to bridge from.
type StringEncoding int

// EncUTF8, EncUTF16, EncLatin1UTF16 are the three canonical-ABI string
// encoding modes.
const (
	EncUTF8        StringEncoding = 0
	EncUTF16       StringEncoding = 1
	EncLatin1UTF16 StringEncoding = 2
)

// ReallocFunc is the wazero-backed callee realloc, retyped to avoid a wazero
// dependency at this layer.
type ReallocFunc func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error)

// PostReturnFunc is the wazero-backed callee post-return hook signature,
// retyped to avoid a wazero dependency at this layer.
type PostReturnFunc func(ctx context.Context, results []uint64) error

// Task is the per-call accounting record. Spec terminology — every call
// is a task that scopes the borrows it lifts. Lifetime equals the
// duration of one call (transfer or gocall).
type Task struct {
	// NumBorrows is the live callee-side borrow count for this task.
	// Public during migration; new code uses BorrowIssued/BorrowDropped/
	// End. Will be unexported once all call sites use the methods.
	NumBorrows uint32
	releases   []func()
}

// BorrowIssued increments the borrow count.
func (t *Task) BorrowIssued() { t.NumBorrows++ }

// BorrowDropped decrements the borrow count. Underflow is treated as a bug
// — silently clamped at zero so a stale drop after an error-path teardown
// cannot wrap to 0xFFFFFFFF and poison later accounting.
func (t *Task) BorrowDropped() {
	if t.NumBorrows == 0 {
		return
	}
	t.NumBorrows--
}

// AddRelease registers fn to fire on End. Used by LendTo to attach the
// matching unlend for a lifted borrow.
func (t *Task) AddRelease(fn func()) {
	t.releases = append(t.releases, fn)
}

// End drains all registered releases (in registration order) and returns
// an error if NumBorrows is non-zero. Always called from a defer at end
// of call.
func (t *Task) End() error {
	for _, fn := range t.releases {
		fn()
	}
	t.releases = nil
	if t.NumBorrows > 0 {
		return fmt.Errorf("call returned with %d outstanding borrows", t.NumBorrows)
	}
	return nil
}

// MaxListByteLength is the maximum total byte size for a list (per canonical
// ABI spec, 2^28 - 1).
const MaxListByteLength uint32 = (1 << 28) - 1

// alignUp rounds v up to the next multiple of a. a==0 is a no-op.
// Traps if v + a - 1 would wrap uint32 — adversarial layouts cannot collide
// fields by overflowing the running offset.
func alignUp(v, a uint32) uint32 {
	if a == 0 {
		return v
	}
	if v > 0xFFFFFFFF-(a-1) {
		trapf("type layout overflow: alignUp(%d, %d)", v, a)
	}
	return (v + a - 1) & ^(a - 1)
}

// trapf panics with a *Trap carrying the formatted message. Used by
// string decoding and other helpers that fail fatally mid-transfer.
func trapf(format string, args ...any) {
	panic(&Trap{msg: fmtSprintf(format, args...)})
}

// callRealloc invokes a ReallocFunc with runtime safety checks: nil func,
// null return, misalignment, and out-of-bounds pointer all trap.
func callRealloc(ctx context.Context, r ReallocFunc, mem api.Memory, origPtr, origSize, align, newSize uint32) (uint32, error) {
	if r == nil {
		trapf("realloc required but not provided")
	}
	ptr, err := r(ctx, origPtr, origSize, align, newSize)
	if err != nil {
		return 0, err
	}
	if ptr == 0 {
		trapf("realloc returned null")
	}
	if align != 0 && ptr%align != 0 {
		trapf("realloc returned unaligned pointer %d (align %d)", ptr, align)
	}
	if uint64(ptr)+uint64(newSize) > uint64(mem.Size()) {
		trapf("realloc return: beyond end of memory: pointer %d+%d (mem size %d)", ptr, newSize, mem.Size())
	}
	return ptr, nil
}

// memShim adapts api.Memory to the fakeOrRealMemory interface used by
// string decoding helpers.
type memShim struct{ m api.Memory }

func (s memShim) Read(off, length uint32) ([]byte, bool) { return s.m.Read(off, length) }
