package canon

import (
	"errors"
	"fmt"
)

// ErrLiftLimitExceeded is wrapped by the error reported when a single lift
// (one call's params, or one call's results) would materialize more bytes
// than the engine's configured maximum.
var ErrLiftLimitExceeded = errors.New("lift size limit exceeded")

// LiftBudget bounds the bytes one lift may materialize in its receiver.
// Each lift path charges what it allocates: a transfer charges the bytes
// it reallocs in the receiver's memory (strings post-transcoding), and a
// lift into Go Vals charges each list's and string's estimated Go
// footprint (see valFootprint). A zero limit disables accounting.
//
// Generated decoders charge through CallContext.ChargeLift: CallBinding.CallRaw
// supplies the result budget, and host callbacks start an argument budget.
type LiftBudget struct {
	limit uint64
	used  uint64
}

// NewLiftBudget starts an independent lift with the given byte limit.
// A zero limit disables accounting.
func NewLiftBudget(limit uint64) LiftBudget { return LiftBudget{limit: limit} }

// reset starts a new lift.
func (b *LiftBudget) reset() { b.used = 0 }

// Charge records n more lifted bytes, returning an error wrapping
// ErrLiftLimitExceeded if the lift's total would exceed the limit. A nil
// budget accepts everything.
func (b *LiftBudget) Charge(n uint64) error {
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
func (b *LiftBudget) mustCharge(n uint64) {
	if err := b.Charge(n); err != nil {
		panic(&Trap{msg: err.Error(), err: err})
	}
}

// checkLiftedList validates a list header about to be lifted into Go
// values and charges the list's Go footprint (elemFootprint per element,
// plus its slot) to the call's budget. It runs before any Go-side
// allocation so a hostile length cannot force one.
func checkLiftedList(gcc *gocallContext, ptr, n, elemSize, elemAlign uint32, elemFootprint uint64) error {
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
	return gcc.budget.Charge(listElemCharge(n, elemFootprint))
}
