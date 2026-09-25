package witgen

import (
	"strings"
	"testing"
)

func TestEmitLiftFlat_String(t *testing.T) {
	body := helperBody(TypeString{}, modeLiftFlat)
	for _, want := range []string{
		"uint32(stack[0])",
		"uint32(stack[1])",
		"callee.Memory().Read(",
		"string(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftFlatString body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLowerFlat_String(t *testing.T) {
	body := helperBody(TypeString{}, modeLowerFlat)
	for _, want := range []string{
		"[]byte(v)",
		`callee.Realloc(ctx,`,
		"stack[0] = uint64(",
		"stack[1] = uint64(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("lowerFlatString body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLiftMem_String(t *testing.T) {
	body := helperBody(TypeString{}, modeLiftMem)
	for _, want := range []string{
		"ReadUint32Le(ptr)",
		"ReadUint32Le(ptr + 4)",
		"callee.Memory().Read(",
		"string(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftMemString body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLowerMem_String(t *testing.T) {
	body := helperBody(TypeString{}, modeLowerMem)
	for _, want := range []string{
		"[]byte(v)",
		`callee.Realloc(ctx,`,
		"WriteUint32Le(ptr,",
		"WriteUint32Le(ptr + 4,",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("lowerMemString body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLiftFlat_ListU32(t *testing.T) {
	list := &TypeList{Elem: PrimU32}
	body := helperBody(list, modeLiftFlat)
	for _, want := range []string{
		"uint32(stack[0])",
		"uint32(stack[1])",
		"make([]uint32",
		// One bounds-checked view of the whole list, then a decode loop.
		"callee.Memory().Read(ptr_, uint32(n_))",
		"binary.LittleEndian.Uint32(buf_[o_:])",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftFlatListU32 body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLowerFlat_ListU32(t *testing.T) {
	list := &TypeList{Elem: PrimU32}
	body := helperBody(list, modeLowerFlat)
	for _, want := range []string{
		`callee.Realloc(ctx,`,
		"len(v)",
		"callee.Memory().Read(ptr_, uint32(n_))",
		"binary.LittleEndian.PutUint32(buf_[o_:], e_)",
		"stack[0] = uint64(ptr_)",
		"stack[1] = uint64(ln_)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("lowerFlatListU32 body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLiftMem_TupleU32String(t *testing.T) {
	tup := &TypeTuple{Fields: []Type{PrimU32, TypeString{}}}
	body := helperBody(tup, modeLiftMem)
	for _, want := range []string{
		"ReadUint32Le(ptr)", // u32 at offset 0
		"liftMemString(ctx, caller, callee, h, tupT_.Types[1], ptr + 4)", // string at offset 4
		"v_.F0",
		"v_.F1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftMemTupleU32String body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLowerMem_TupleU32String(t *testing.T) {
	tup := &TypeTuple{Fields: []Type{PrimU32, TypeString{}}}
	body := helperBody(tup, modeLowerMem)
	for _, want := range []string{
		"WriteUint32Le(ptr, v.F0)",
		"lowerMemString(ctx, caller, callee, h, tupT_.Types[1], ptr + 4, v.F1)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("lowerMemTupleU32String body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLiftFlat_TupleU32String(t *testing.T) {
	// Tuple in flat mode: u32 at slot 0, string at slots 1..2.
	tup := &TypeTuple{Fields: []Type{PrimU32, TypeString{}}}
	body := helperBody(tup, modeLiftFlat)
	for _, want := range []string{
		"v_.F0 = uint32(stack[0])",
		"liftFlatString(ctx, caller, callee, h, tupT_.Types[1], stack[1:3])",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftFlatTupleU32String body missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitHelper_FunctionSignatures(t *testing.T) {
	// Spot-check the wrapper signatures emitted by emitHelper.
	var sb strings.Builder
	emitHelper(&sb, TypeString{}, modeLiftFlat)
	emitHelper(&sb, TypeString{}, modeLiftMem)
	emitHelper(&sb, TypeString{}, modeLowerFlat)
	emitHelper(&sb, TypeString{}, modeLowerMem)
	out := sb.String()
	for _, want := range []string{
		"func liftFlatString(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, stack []uint64) (string, error) {",
		"func liftMemString(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, ptr uint32) (string, error) {",
		"func lowerFlatString(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, stack []uint64, v string) error {",
		"func lowerMemString(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, ptr uint32, v string) error {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("emitted helpers missing signature %q. Output:\n%s", want, out)
		}
	}
}

func TestEmitWrapFunc_StringResultGoesToMem(t *testing.T) {
	// Function with a string parameter and a string result. String has
	// FlatSlots=2 > 1, so result goes through mem-mode.
	fn := &Func{
		WitName: "echo",
		GoName:  "Echo",
		Params:  []*Param{{GoName: "s", Type: TypeString{}}},
		Result:  TypeString{},
	}
	var sb strings.Builder
	emitWrapFunc(&sb, fn)
	out := sb.String()
	for _, want := range []string{
		"func wrapEcho(f *Factory) host.Func {",
		"toGoFlatString(ctx, cc, h, fnType_.Params[0].Type, stack[0:2])",
		"result_, implErr_ := state_.impl.Echo(ctx, s)",
		`outPtr_, rerr_ := cc.Realloc(ctx,`,
		"fromGoMemString(ctx, cc, h, outPtr_, result_)",
		"stack[0] = uint64(outPtr_)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wrapEcho missing %q. Output:\n%s", want, out)
		}
	}
}

func TestEmitLiftFlat_OptionU32(t *testing.T) {
	body := helperBody(&TypeOption{Elem: PrimU32}, modeLiftFlat)
	for _, want := range []string{
		"uint32(stack[0])", // read discriminant
		"var out_ OptionU32",
		"out_.IsSome = true",
		"out_.Value",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftFlatOptionU32 missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLowerFlat_OptionU32(t *testing.T) {
	body := helperBody(&TypeOption{Elem: PrimU32}, modeLowerFlat)
	for _, want := range []string{
		"if v.IsSome",
		"stack[0] = 1", // discriminant = 1 when Some
		"stack[1]",     // payload written to slot 1
	} {
		if !strings.Contains(body, want) {
			t.Errorf("lowerFlatOptionU32 missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLiftFlat_ResultU32String(t *testing.T) {
	// Sealed Result shape: lift returns ResultU32StringOk{Value:...} or
	// ResultU32StringErr{Value:...}.
	body := helperBody(&TypeResult{OK: PrimU32, Err: TypeString{}}, modeLiftFlat)
	for _, want := range []string{
		"uint32(stack[0])", // discriminant
		"ResultU32StringOk{Value:",
		"ResultU32StringErr{Value:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftFlatResultU32String missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitLiftMem_OptionU32(t *testing.T) {
	body := helperBody(&TypeOption{Elem: PrimU32}, modeLiftMem)
	for _, want := range []string{
		"ReadByte(ptr)", // discriminant byte
		"ptr + 4",       // payload offset (1 disc + 3 pad aligned to 4)
		"var out_ OptionU32",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("liftMemOptionU32 missing %q. Body:\n%s", want, body)
		}
	}
}

func TestEmitWrapFunc_ResultReturnLowersSealed(t *testing.T) {
	// The impl returns the sealed Result type directly; the trampoline
	// lowers it without synthesizing the discriminant from (T, error).
	fn := &Func{
		WitName: "parse",
		GoName:  "Parse",
		Params:  []*Param{{GoName: "s", Type: TypeString{}}},
		Result:  &TypeResult{OK: PrimU32, Err: TypeString{}},
	}
	var w strings.Builder
	emitWrapFunc(&w, fn)
	src := w.String()
	for _, want := range []string{
		"result_, implErr_ := state_.impl.Parse(ctx, s)",
		`return fmt.Errorf("wacogo/witgen: parse: impl returned error: %w", implErr_)`,
		"fromGoMemResultU32String(ctx, cc, h, outPtr_, result_)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
	// No legacy synthesis locals.
	for _, unwanted := range []string{
		"var r ResultU32String",
		"ResultU32StringErr{Value: err.Error()}",
	} {
		if strings.Contains(src, unwanted) {
			t.Errorf("unexpected %q. Source:\n%s", unwanted, src)
		}
	}
}

func TestEmitWrapFunc_ResultErrorOnly(t *testing.T) {
	// Function returning result (both arms void) → Go sig is (Result__, error).
	// Impl returns the sealed value directly.
	fn := &Func{
		WitName: "validate",
		GoName:  "Validate",
		Result:  &TypeResult{},
	}
	var w strings.Builder
	emitWrapFunc(&w, fn)
	src := w.String()
	for _, want := range []string{
		"result_, implErr_ := state_.impl.Validate(ctx)",
		`return fmt.Errorf("wacogo/witgen: validate: impl returned error: %w", implErr_)`,
		"fromGoFlatResult__(ctx, cc, h, stack[0:1], result_)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
}

func TestEmitWrapFunc_EnumParam(t *testing.T) {
	enum := &TypeEnum{Name: "color", GoName: "Color", Cases: []EnumCase{
		{Name: "red", GoName: "ColorRed"},
	}}
	fn := &Func{
		WitName: "echo",
		GoName:  "Echo",
		Params:  []*Param{{GoName: "c", Type: enum}},
		Result:  enum,
	}
	var w strings.Builder
	emitWrapFunc(&w, fn)
	src := w.String()
	for _, want := range []string{
		"c := Color(",
		"result_, implErr_ := state_.impl.Echo(ctx, c)",
		"stack[0] = uint64(result_)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
}

func TestEmitWrapFunc_TupleResultGoesToMem(t *testing.T) {
	// Tuple<u32, string> has 3 flat slots > 1 → mem-mode result.
	tup := &TypeTuple{Fields: []Type{PrimU32, TypeString{}}}
	fn := &Func{
		WitName: "pair",
		GoName:  "Pair",
		Result:  tup,
	}
	var sb strings.Builder
	emitWrapFunc(&sb, fn)
	out := sb.String()
	for _, want := range []string{
		`outPtr_, rerr_ := cc.Realloc(ctx,`,
		"fromGoMemTupleU32String(ctx, cc, h, outPtr_, result_)",
		"stack[0] = uint64(outPtr_)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wrapPair missing %q. Output:\n%s", want, out)
		}
	}
}

func TestEmitWrapCtor(t *testing.T) {
	rt := &TypeResource{
		Name: "counter", GoName: "Counter", WrapName: "Counter", ImplName: "CounterImpl",
		Ctor: &ResourceCtor{
			Params: []*Param{{GoName: "initial", Type: PrimU32}},
		},
	}
	var w strings.Builder
	emitWrapCtor(&w, rt)
	src := w.String()
	for _, want := range []string{
		"func wrapNewCounter(f *Factory) host.Func",
		"func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64)",
		"state_ := h.UserState().(*instanceState)",
		"result_, err_ := state_.impl.NewCounter(ctx, initial)",
		`return fmt.Errorf("wacogo/witgen: [constructor]counter: impl returned error: %w", err_)`,
		"oh_, err_ := result_.bind(cc, h)",
		`return fmt.Errorf("wacogo/witgen: [constructor]counter: bind failed: %w", err_)`,
		"result_.Invalidate()",
		"stack[0] = uint64(oh_)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
}

func TestEmitWrapMethod(t *testing.T) {
	rt := &TypeResource{
		Name: "counter", GoName: "Counter", WrapName: "Counter", ImplName: "CounterImpl",
	}
	m := ResourceMethod{
		Name: "increment", GoName: "Increment",
	}
	var w strings.Builder
	emitWrapMethod(&w, rt, m)
	src := w.String()
	for _, want := range []string{
		"func wrapCounterIncrement(f *Factory) host.Func",
		"func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64)",
		"selfObj_, ok_ := h.LookupResource(host.ExternHandle(uint32(stack[0])))",
		"self_ := selfObj_.(Counter)",
		"self_.Increment(ctx)",
		`return fmt.Errorf("wacogo/witgen: [method]counter.increment: impl returned error: %w", err_)`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
}

func TestEmitWrapMethod_U32Result(t *testing.T) {
	rt := &TypeResource{Name: "counter", GoName: "Counter", WrapName: "Counter", ImplName: "CounterImpl"}
	m := ResourceMethod{
		Name: "current", GoName: "Current",
		Result: PrimU32,
	}
	var w strings.Builder
	emitWrapMethod(&w, rt, m)
	src := w.String()
	for _, want := range []string{
		"func wrapCounterCurrent(f *Factory) host.Func",
		"func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64)",
		"selfObj_, ok_ := h.LookupResource(host.ExternHandle(uint32(stack[0])))",
		"self_ := selfObj_.(Counter)",
		"result_, implErr_ := self_.Current(ctx)",
		"stack[0] = uint64(result_)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
}

func TestEmitWrapStatic(t *testing.T) {
	rt := &TypeResource{Name: "counter", GoName: "Counter", WrapName: "Counter", ImplName: "CounterImpl"}
	s := ResourceStatic{
		Name:   "open",
		GoName: "CounterOpen",
		Params: []*Param{{GoName: "name", Type: TypeString{}}},
		Result: PrimU32,
	}
	var w strings.Builder
	emitWrapStatic(&w, rt, s)
	src := w.String()
	for _, want := range []string{
		"func wrapCounterOpen(f *Factory) host.Func",
		"toGoFlatString(ctx, cc, h, fnType_.Params[0].Type, stack[0:2])",
		"result_, implErr_ := state_.impl.CounterOpen(ctx, name)",
		"stack[0] = uint64(result_)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q. Source:\n%s", want, src)
		}
	}
}
