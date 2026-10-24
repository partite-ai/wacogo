package canon

import (
	"errors"
	"fmt"
)

// ErrLiftLimitExceeded is wrapped by the error reported when a single lift
// (one call's params, or one call's results) would materialize more bytes
// than the engine's configured maximum.
var ErrLiftLimitExceeded = errors.New("lift size limit exceeded")

// liftBudget bounds the bytes one lift may materialize in its receiver.
// Charges are canonical-ABI sizes: n*elemSize per list, the byte length per
// string (post-transcoding when the receiver's realloc is involved). A zero
// limit disables accounting.
type liftBudget struct {
	limit uint64
	used  uint64
}

// reset starts a new lift.
func (b *liftBudget) reset() { b.used = 0 }

// charge records n more lifted bytes, returning an error wrapping
// ErrLiftLimitExceeded if the lift's total would exceed the limit. A nil
// budget accepts everything.
func (b *liftBudget) charge(n uint64) error {
	if b == nil || b.limit == 0 {
		return nil
	}
	// used never exceeds limit, so limit-used cannot underflow.
	if n > b.limit-b.used {
		return fmt.Errorf("%w: lifted value exceeds %d bytes", ErrLiftLimitExceeded, b.limit)
	}
	b.used += n
	return nil
}

// mustCharge is charge for trap-based call paths.
func (b *liftBudget) mustCharge(n uint64) {
	if err := b.charge(n); err != nil {
		panic(&Trap{msg: err.Error(), err: err})
	}
}

// checkLiftedList validates a list header about to be lifted into Go
// values and charges its canonical size to the call's budget. It runs
// before any Go-side allocation so a hostile length cannot force one.
func checkLiftedList(gcc *gocallContext, ptr, n, elemSize, elemAlign uint32) error {
	if err := checkListLen(n, elemSize); err != nil {
		return err
	}
	if n > 0 && elemAlign > 1 {
		if err := checkAlignment(ptr, elemAlign); err != nil {
			return err
		}
	}
	byteLen := uint64(n) * uint64(elemSize)
	if uint64(ptr)+byteLen > uint64(gcc.callee.Memory.Size()) {
		return fmt.Errorf("list pointer/length out of bounds of memory: ptr=%d len=%d", ptr, byteLen)
	}
	return gcc.budget.charge(byteLen)
}
