package canon

import (
	"context"
)

// transferPlanStep is the closure type emitted by transfer visitors.
// srcBase and dstBase are byte addresses in caller/callee linear memory.
// Flat-mode step closures ignore both and use compile-time absolute slot
// indices. Mem-mode steps add their compile-time byte offset to the base
// passed in (no context-state lookup).
type transferPlanStep func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32)

// gocallLowerStep is the closure type emitted by val→wasm visitors
// (valToFlatVisitor, valToMemVisitor). The input Val is passed directly;
// composite outer steps destructure it and forward sub-Vals to their
// field/element/case sub-steps. Flat-mode steps ignore base and write
// gcc.registers at compile-time absolute slots; mem-mode steps write
// gcc.callee.Memory at base + compile-time offset.
type gocallLowerStep func(ctx context.Context, gcc *gocallContext, v Val, base uint32) error

// gocallLiftStep is the closure type emitted by wasm→val visitors
// (flatToValVisitor, memToValVisitor). Each step returns the lifted Val;
// composite outer steps combine child Vals into the enclosing shape.
// Flat-mode steps ignore base and read gcc.registers at compile-time
// absolute slots; mem-mode steps read gcc.callee.Memory at base + offset.
type gocallLiftStep func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error)

// transferPlan is the compiled recipe for a component→component call.
type transferPlan struct {
	paramMemSize   uint32 // 0 in flat params mode
	paramMaxAlign  uint32 // 0 in flat params mode
	returnMem      bool   // true ⇒ callee returns a single i32 ptr (canon-lift)
	resultMaxAlign uint32 // max alignment across mem-mode result fields (0 when !returnMem)
	paramSteps     []transferPlanStep
	resultSteps    []transferPlanStep
}

// gocallPlan is the compiled recipe for a Go→component call (Func.Call).
//
// paramSteps holds exactly one step per top-level param type; the runner
// calls each with args[i] as the input Val. resultSteps holds exactly one
// step per top-level result type; the runner collects each returned Val
// into the output []Val.
type gocallPlan struct {
	paramMemSize   uint32
	paramMaxAlign  uint32
	nParamRegs     int // core-args slice size: flat-slot count, or 1 for mem params
	nResultRegs    int // flat-mode: sum of flat slots for all results (≤1 per maxFlatResults cap); mem-mode: 1 (the result-block pointer)
	returnMem      bool
	resultMaxAlign uint32
	paramSteps     []gocallLowerStep
	resultSteps    []gocallLiftStep
}

// maxFlatParams / maxFlatResults per canonical ABI spec.
const (
	maxFlatParams  = 16
	maxFlatResults = 1
)

// pickFlat returns true if the total flat slot count of ts fits within max.
func pickFlat(ts []Type, max uint32) bool {
	var n uint32
	for _, t := range ts {
		v := &flatCountVisitor{}
		t.Accept(v)
		n += v.count
	}
	return n <= max
}

// compileTransferPlan builds a transferPlan for a component↔component call.
// The same-component shortcut (lower_borrow skipped when borrow's defining
// instance equals the callee) is handled inside LendTo on the ResourceHandle,
// so no calleeInstance parameter is needed here.
func compileTransferPlan(params, results []Type, _ Instance) *transferPlan {
	plan := &transferPlan{}

	if pickFlat(params, maxFlatParams) {
		fv := &flatTransferVisitor{}
		for _, t := range params {
			t.Accept(fv)
		}
		plan.paramSteps = fv.out
	} else {
		mv := &memTransferVisitor{}
		for _, t := range params {
			t.Accept(mv)
		}
		plan.paramSteps = mv.out
		plan.paramMemSize = alignUp(mv.byteOff, mv.maxAlign)
		plan.paramMaxAlign = mv.maxAlign
	}

	if pickFlat(results, maxFlatResults) {
		fv := &flatTransferVisitor{}
		for _, t := range results {
			t.Accept(fv)
		}
		plan.resultSteps = fv.out
	} else {
		mv := &memTransferVisitor{}
		for _, t := range results {
			t.Accept(mv)
		}
		plan.resultSteps = mv.out
		plan.returnMem = true
		plan.resultMaxAlign = mv.maxAlign
	}

	return plan
}

// compileGocallPlan builds a gocallPlan for a Go→component call (Func.Call).
// Params and results mode are selected independently per canonical ABI spec.
// Each top-level type's Accept emits exactly one step appended to the
// visitor's out slice, so paramSteps and resultSteps parallel params and
// results element-for-element.
func compileGocallPlan(params, results []Type) *gocallPlan {
	plan := &gocallPlan{}

	if pickFlat(params, maxFlatParams) {
		fv := &valToFlatVisitor{}
		for _, t := range params {
			t.Accept(fv)
		}
		plan.paramSteps = fv.out
		var flat uint32
		for _, t := range params {
			flat += flatCountForType(t)
		}
		plan.nParamRegs = int(flat)
	} else {
		mv := &valToMemVisitor{}
		for _, t := range params {
			t.Accept(mv)
		}
		plan.paramSteps = mv.out
		plan.paramMemSize = alignUp(mv.byteOff, mv.maxAlign)
		plan.paramMaxAlign = mv.maxAlign
		plan.nParamRegs = 1 // just the callee-side param block pointer
	}

	if pickFlat(results, maxFlatResults) {
		fv := &flatToValVisitor{}
		for _, t := range results {
			t.Accept(fv)
		}
		plan.resultSteps = fv.out
		var flat uint32
		for _, t := range results {
			flat += flatCountForType(t)
		}
		plan.nResultRegs = int(flat)
	} else {
		mv := &memToValVisitor{}
		for _, t := range results {
			t.Accept(mv)
		}
		plan.resultSteps = mv.out
		plan.nResultRegs = 1 // callee returns a single i32 result-block pointer
		plan.returnMem = true
		plan.resultMaxAlign = mv.maxAlign
	}

	return plan
}

// flatCount returns the flat-mode slot count of t.
func flatCount(t Type) uint32 { return flatCountForType(t) }
