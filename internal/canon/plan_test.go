package canon

import (
	"testing"
)

func TestCompileTransferPlanFlat(t *testing.T) {
	// Two u32 params, one u32 result → all flat mode.
	params := []Type{testU32Type{}, testU32Type{}}
	results := []Type{testU32Type{}}
	plan := compileTransferPlan(params, results, nil)
	if plan.paramMemSize != 0 {
		t.Fatalf("expected flat params; got paramMemSize=%d", plan.paramMemSize)
	}
	if plan.returnMem {
		t.Fatal("expected flat results")
	}
	if len(plan.paramSteps) != 2 {
		t.Fatalf("expected 2 param steps, got %d", len(plan.paramSteps))
	}
	if len(plan.resultSteps) != 1 {
		t.Fatalf("expected 1 result step, got %d", len(plan.resultSteps))
	}
}

func TestCompileTransferPlanMemResultViaTwoU32(t *testing.T) {
	// Two u32 results → 2 flat slots exceeds maxFlatResults=1 → mem mode.
	params := []Type{}
	results := []Type{testU32Type{}, testU32Type{}}
	plan := compileTransferPlan(params, results, nil)
	if !plan.returnMem {
		t.Fatal("expected mem results")
	}
	if len(plan.resultSteps) != 2 {
		t.Fatalf("got %d result steps", len(plan.resultSteps))
	}
}

func TestCompileTransferPlanMemParamsViaManyU32(t *testing.T) {
	// 17 u32 params → exceeds maxFlatParams=16 → mem mode.
	params := make([]Type, 17)
	for i := range params {
		params[i] = testU32Type{}
	}
	plan := compileTransferPlan(params, nil, nil)
	if plan.paramMemSize == 0 {
		t.Fatal("expected mem params; got flat")
	}
	// 17 u32s at 4 bytes each = 68 bytes
	if plan.paramMemSize != 68 {
		t.Fatalf("expected paramMemSize=68, got %d", plan.paramMemSize)
	}
	if plan.paramMaxAlign != 4 {
		t.Fatalf("expected paramMaxAlign=4, got %d", plan.paramMaxAlign)
	}
}

func TestCompileGocallPlanFlat(t *testing.T) {
	// Single u32 param, single u32 result → all flat.
	params := []Type{testU32Type{}}
	results := []Type{testU32Type{}}
	plan := compileGocallPlan(params, results)
	if plan.paramMemSize != 0 {
		t.Fatalf("expected flat params")
	}
	if len(plan.paramSteps) != 1 || len(plan.resultSteps) != 1 {
		t.Fatalf("step counts wrong: params=%d results=%d",
			len(plan.paramSteps), len(plan.resultSteps))
	}
}

func TestCompileGocallPlanMemParams(t *testing.T) {
	params := make([]Type, 17)
	for i := range params {
		params[i] = testU32Type{}
	}
	plan := compileGocallPlan(params, nil)
	if plan.paramMemSize == 0 {
		t.Fatal("expected mem params")
	}
}
