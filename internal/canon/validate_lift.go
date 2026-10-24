package canon

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// LiftValidator preflights canonical host arguments using the same layout and
// traversal as result lifting. It never copies strings, builds collections, or
// transfers handles. Its compiled steps are immutable and concurrent-safe.
// Storage charges cover generated Go layouts, including inactive inline payloads.
type LiftValidator struct {
	steps    []gocallLiftStep
	indirect bool
	align    uint32
	limits   LiftLimits
}

func NewLiftValidator(params []Type, limits LiftLimits) *LiftValidator {
	v := &LiftValidator{limits: limits}
	if pickFlat(params, maxFlatParams) {
		visitor := &flatToValVisitor{}
		for _, param := range params {
			param.Accept(visitor)
		}
		v.steps = visitor.out
	} else {
		visitor := &memToValVisitor{}
		for _, param := range params {
			param.Accept(visitor)
		}
		v.steps = visitor.out
		v.indirect = true
		v.align = visitor.maxAlign
	}
	return v
}

func (v *LiftValidator) Validate(ctx context.Context, memory api.Memory, encoding StringEncoding, stack []uint64) (err error) {
	return v.validate(ctx, memory, encoding, stack, false)
}

// ValidateTransfer also charges transcoding workspace and the final UTF-8 host
// copy. It runs on source memory before a component-to-host transfer starts.
func (v *LiftValidator) ValidateTransfer(ctx context.Context, memory api.Memory, encoding StringEncoding, stack []uint64) error {
	return v.validate(ctx, memory, encoding, stack, true)
}

func (v *LiftValidator) validate(ctx context.Context, memory api.Memory, encoding StringEncoding, stack []uint64, transfer bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if trap, ok := r.(*Trap); ok {
				err = fmt.Errorf("%s", trap.Error())
				return
			}
			panic(r)
		}
	}()
	gcc := gocallContext{
		callee:           &transferSide{Memory: memory, StringEncoding: encoding},
		registers:        stack,
		lift:             liftBudget{limits: v.limits},
		validateOnly:     true,
		generatedStorage: true,
		transferToHost:   transfer,
	}
	var base uint32
	if v.indirect {
		base = uint32(stack[0])
		if err := checkAlignment(base, v.align); err != nil {
			return err
		}
	}
	for _, step := range v.steps {
		if _, err := step(ctx, &gcc, base); err != nil {
			return err
		}
	}
	return nil
}
