package canon

import (
	"context"
	"errors"
	"fmt"
	"unsafe"
)

// LiftLimits bounds canonical values lifted into Go: Func.Call results and
// arguments passed to host functions. Each zero field is unlimited.
// CallRaw callers, including generated typed bindings, manage their own result
// decoding and allocation limits. Generated results are not covered.
type LiftLimits struct {
	// MaxValues limits the aggregate number of values, including nested values.
	MaxValues uint64
	// MaxBytes limits cumulative string copies and collection backing storage.
	// Encoding conversion workspace is charged conservatively before allocation.
	// Fixed-size value objects are bounded separately by MaxValues; this field
	// is not a process heap limit. Repeated aliases are charged independently.
	MaxBytes uint64
}

// ErrLiftLimit reports that a value exceeded its configured lifting budget.
var ErrLiftLimit = errors.New("canonical value exceeds lifting limit")

const (
	valStorage   = uint64(unsafe.Sizeof([1]Val{}))
	fieldStorage = uint64(unsafe.Sizeof(Field{}))
)

type liftBudget struct {
	limits        LiftLimits
	values, bytes uint64
}

func (b *liftBudget) reserveValues(count uint64) error {
	if b.limits.MaxValues != 0 && count > b.limits.MaxValues-b.values {
		return fmt.Errorf("%w: more than %d values", ErrLiftLimit, b.limits.MaxValues)
	}
	return nil
}

func (b *liftBudget) value(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.reserveValues(1); err != nil {
		return err
	}
	if b.limits.MaxValues != 0 {
		b.values++
	}
	return nil
}

func (b *liftBudget) storage(count uint64) error {
	if b.limits.MaxBytes != 0 {
		if count > b.limits.MaxBytes-b.bytes {
			return fmt.Errorf("%w: more than %d storage bytes", ErrLiftLimit, b.limits.MaxBytes)
		}
		b.bytes += count
	}
	return nil
}

func (b *liftBudget) stringStorage(data []byte, coded uint32, enc StringEncoding, transfer bool) error {
	multiplier := uint64(1)
	switch enc {
	case EncUTF16:
		// UTF-16 units, decoded runes, and UTF-8 output.
		multiplier = 5
		if transfer {
			multiplier = 8
		}
	case EncLatin1UTF16:
		multiplier = 6 // Latin-1 runes and UTF-8 output.
		if transfer {
			multiplier = 10
		}
		if _, utf16 := latin1UTF16Decode(coded); utf16 {
			multiplier = 5
			if transfer {
				multiplier = 8
			}
		}
	}
	return b.storage(uint64(len(data)) * multiplier)
}

func (gcc *gocallContext) listStorage(ptr, count, elemSize uint32) error {
	if err := checkListLen(count, elemSize); err != nil {
		return err
	}
	if count != 0 && elemSize != 0 {
		if _, ok := gcc.callee.Memory.Read(ptr, count*elemSize); !ok {
			return errors.New("list pointer/length out of bounds of memory")
		}
	}
	if err := gcc.lift.reserveValues(uint64(count)); err != nil {
		return err
	}
	return gcc.lift.storage(uint64(count) * valStorage)
}

func (gcc *gocallContext) recordStorage(count int) error {
	if err := gcc.lift.reserveValues(uint64(count)); err != nil {
		return err
	}
	// The field slice and NewValRecord's defensive copy are separate allocations.
	return gcc.lift.storage(uint64(count) * fieldStorage * 2)
}

func liftStep(step gocallLiftStep) gocallLiftStep {
	return func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		if err := gcc.lift.value(ctx); err != nil {
			return nil, err
		}
		return step(ctx, gcc, base)
	}
}

func (gcc *gocallContext) takeOwn(rt ResourceType, handle uint32) (Val, error) {
	// Tracking slice growth may reserve up to twice its pointer count.
	if err := gcc.lift.storage(2 * uint64(unsafe.Sizeof((*ValOwnHandle)(nil)))); err != nil {
		return nil, err
	}
	source, err := gcc.callee.ResourceTable.LookupOwn(rt, handle)
	if err != nil {
		return nil, err
	}
	value := newValOwnHandleEmpty()
	if _, err := source.TransferOwn(value); err != nil {
		return nil, err
	}
	gcc.owned = append(gcc.owned, value)
	return value, nil
}

// finishLift runs after post-return, task releases and callee Exit. Resource
// destructors may reenter their defining instance, so Exit must run first.
func finishLift(ctx context.Context, owned []*ValOwnHandle, results *[]Val, err *error, panicked bool) {
	if *err != nil || panicked {
		for i := len(owned) - 1; i >= 0; i-- {
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						if cause, ok := recovered.(error); ok {
							*err = errors.Join(*err, fmt.Errorf("lift rollback destructor: %w", cause))
						} else {
							*err = errors.Join(*err, fmt.Errorf("lift rollback destructor: %v", recovered))
						}
					}
				}()
				*err = errors.Join(*err, owned[i].Drop(ctx))
			}()
		}
		*results = nil
	}
	clear(owned)
}
