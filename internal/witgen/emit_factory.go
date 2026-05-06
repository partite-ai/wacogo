package witgen

import (
	"bytes"
	"fmt"
	"strings"
)

// bindView is the template input for file_header.tmpl when emitting
// the .bind.go file. Embeds Interface to reuse header template fields.
type bindView struct {
	*Interface
	SourceFile string
}

// liftFlatExpr returns a Go expression that decodes the primitive p
// from the flat-stack slot expression src (e.g., "stack[0]").
func liftFlatExpr(p Prim, src string) string {
	switch p {
	case PrimBool:
		return src + " != 0"
	case PrimU8:
		return "uint8(" + src + ")"
	case PrimU16:
		return "uint16(" + src + ")"
	case PrimU32:
		return "uint32(" + src + ")"
	case PrimU64:
		return src
	case PrimS8:
		return "int8(int64(" + src + "))"
	case PrimS16:
		return "int16(int64(" + src + "))"
	case PrimS32:
		return "int32(int64(" + src + "))"
	case PrimS64:
		return "int64(" + src + ")"
	case PrimF32:
		return "math.Float32frombits(uint32(" + src + "))"
	case PrimF64:
		return "math.Float64frombits(" + src + ")"
	case PrimChar:
		return "rune(" + src + ")"
	}
	panic("witgen: unknown Prim")
}

// lowerFlatExpr returns a Go expression that encodes the value
// expression v of primitive p into a uint64 flat slot value.
func lowerFlatExpr(p Prim, v string) string {
	switch p {
	case PrimBool:
		return "func() uint64 { if " + v + " { return 1 }; return 0 }()"
	case PrimU8, PrimU16, PrimU32:
		return "uint64(" + v + ")"
	case PrimU64:
		return v
	case PrimS8, PrimS16, PrimS32:
		return "uint64(uint32(" + v + "))"
	case PrimS64:
		return "uint64(" + v + ")"
	case PrimF32:
		return "uint64(math.Float32bits(" + v + "))"
	case PrimF64:
		return "math.Float64bits(" + v + ")"
	case PrimChar:
		return "uint64(" + v + ")"
	}
	panic("witgen: unknown Prim")
}

// primMemReadStmt emits a Go statement block that reads a primitive
// value from memExpr at ptrExpr into Go variable dstVar, checking the
// read's error. On read failure, emits the failure-return form
// produced by sink. dstVar must already be declared by the caller;
// the emitter writes only the read+check.
//
// memExpr is the Go expression for the wire-side memory: typically
// "cc.Memory()" for the trampoline-side toGo/fromGo family or
// "callee.Memory()" for the wrap-side lift/lower family.
//
// helperName/op parameterize the wrapped error message ("witgen:
// liftMemX: read field f0: ...").
func primMemReadStmt(p Prim, dstVar, ptrExpr, memExpr, helperName, op string, sink errSink) string {
	var readCall, castExpr string
	switch p {
	case PrimBool:
		readCall = fmt.Sprintf("%s.ReadByte(%s)", memExpr, ptrExpr)
		castExpr = "tmp_ != 0"
	case PrimU8:
		readCall = fmt.Sprintf("%s.ReadByte(%s)", memExpr, ptrExpr)
		castExpr = "tmp_"
	case PrimU16:
		readCall = fmt.Sprintf("%s.ReadUint16Le(%s)", memExpr, ptrExpr)
		castExpr = "tmp_"
	case PrimU32:
		readCall = fmt.Sprintf("%s.ReadUint32Le(%s)", memExpr, ptrExpr)
		castExpr = "tmp_"
	case PrimU64:
		readCall = fmt.Sprintf("%s.ReadUint64Le(%s)", memExpr, ptrExpr)
		castExpr = "tmp_"
	case PrimS8:
		readCall = fmt.Sprintf("%s.ReadByte(%s)", memExpr, ptrExpr)
		castExpr = "int8(tmp_)"
	case PrimS16:
		readCall = fmt.Sprintf("%s.ReadUint16Le(%s)", memExpr, ptrExpr)
		castExpr = "int16(tmp_)"
	case PrimS32:
		readCall = fmt.Sprintf("%s.ReadUint32Le(%s)", memExpr, ptrExpr)
		castExpr = "int32(tmp_)"
	case PrimS64:
		readCall = fmt.Sprintf("%s.ReadUint64Le(%s)", memExpr, ptrExpr)
		castExpr = "int64(tmp_)"
	case PrimF32:
		readCall = fmt.Sprintf("%s.ReadUint32Le(%s)", memExpr, ptrExpr)
		castExpr = "math.Float32frombits(tmp_)"
	case PrimF64:
		readCall = fmt.Sprintf("%s.ReadUint64Le(%s)", memExpr, ptrExpr)
		castExpr = "math.Float64frombits(tmp_)"
	case PrimChar:
		readCall = fmt.Sprintf("%s.ReadUint32Le(%s)", memExpr, ptrExpr)
		castExpr = "rune(tmp_)"
	default:
		panic("witgen: primMemReadStmt: unknown Prim")
	}
	errExpr := fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: %s: bad memory read")`, helperName, op)
	return fmt.Sprintf(`if tmp_, ok_ := %s; !ok_ {
%s
} else {
%s = %s
}`, readCall, sink.Emit(errExpr), dstVar, castExpr)
}

// primMemWriteStmtErr emits a Go statement block that writes a primitive
// value to memExpr at ptrExpr, checking the write's success. On
// failure, emits the failure-return form produced by sink.
//
// memExpr matches primMemReadStmt's: "cc.Memory()" or "callee.Memory()".
func primMemWriteStmtErr(p Prim, ptrExpr, valExpr, memExpr, helperName, op string, sink errSink) string {
	var writeCall string
	switch p {
	case PrimBool:
		writeCall = fmt.Sprintf("%s.WriteByte(%s, func() byte { if %s { return 1 }; return 0 }())", memExpr, ptrExpr, valExpr)
	case PrimU8:
		writeCall = fmt.Sprintf("%s.WriteByte(%s, %s)", memExpr, ptrExpr, valExpr)
	case PrimU16:
		writeCall = fmt.Sprintf("%s.WriteUint16Le(%s, %s)", memExpr, ptrExpr, valExpr)
	case PrimU32:
		writeCall = fmt.Sprintf("%s.WriteUint32Le(%s, %s)", memExpr, ptrExpr, valExpr)
	case PrimU64:
		writeCall = fmt.Sprintf("%s.WriteUint64Le(%s, %s)", memExpr, ptrExpr, valExpr)
	case PrimS8:
		writeCall = fmt.Sprintf("%s.WriteByte(%s, byte(%s))", memExpr, ptrExpr, valExpr)
	case PrimS16:
		writeCall = fmt.Sprintf("%s.WriteUint16Le(%s, uint16(%s))", memExpr, ptrExpr, valExpr)
	case PrimS32:
		writeCall = fmt.Sprintf("%s.WriteUint32Le(%s, uint32(%s))", memExpr, ptrExpr, valExpr)
	case PrimS64:
		writeCall = fmt.Sprintf("%s.WriteUint64Le(%s, uint64(%s))", memExpr, ptrExpr, valExpr)
	case PrimF32:
		writeCall = fmt.Sprintf("%s.WriteUint32Le(%s, math.Float32bits(%s))", memExpr, ptrExpr, valExpr)
	case PrimF64:
		writeCall = fmt.Sprintf("%s.WriteUint64Le(%s, math.Float64bits(%s))", memExpr, ptrExpr, valExpr)
	case PrimChar:
		writeCall = fmt.Sprintf("%s.WriteUint32Le(%s, uint32(%s))", memExpr, ptrExpr, valExpr)
	default:
		panic("witgen: primMemWriteStmtErr: unknown Prim")
	}
	errExpr := fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: %s: bad memory write")`, helperName, op)
	return fmt.Sprintf(`if ok_ := %s; !ok_ {
%s
}`, writeCall, sink.Emit(errExpr))
}

// elemMemReadStmt emits a Go statement block that reads a value of type
// t from memExpr at ptrExpr into dstVar, propagating errors via sink.
// Compounds dispatch into the type's toGoMem (or liftMem) helper —
// the caller picks the family by the helperName prefix it uses in
// recursive emit. memExpr matches primMemReadStmt's convention
// ("cc.Memory()" trampoline-side, "callee.Memory()" wrap-side).
func elemMemReadStmt(t Type, dstVar, ptrExpr, memExpr, helperName, op string, sink errSink) string {
	if p, ok := t.(Prim); ok {
		return primMemReadStmt(p, dstVar, ptrExpr, memExpr, helperName, op, sink)
	}
	if e, ok := t.(*TypeEnum); ok {
		// Enum is a single-byte read with a cast.
		errExpr := fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: %s: bad memory read")`, helperName, op)
		return fmt.Sprintf(`if tmp_, ok_ := %s.ReadByte(%s); !ok_ {
%s
} else {
%s = %s(tmp_)
}`, memExpr, ptrExpr, sink.Emit(errExpr), dstVar, GoTypeOf(e))
	}
	if f, ok := t.(*TypeFlags); ok {
		errExpr := fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: %s: bad memory read")`, helperName, op)
		return fmt.Sprintf(`if tmp_, ok_ := %s.ReadUint32Le(%s); !ok_ {
%s
} else {
%s = %s(tmp_)
}`, memExpr, ptrExpr, sink.Emit(errExpr), dstVar, GoTypeOf(f))
	}
	// Resource handles in compound contexts are handled by the calling
	// helper-emit (toGoMem<X> / liftMem<X>) which knows whether to use
	// the toGo or lift handoff. elemMemReadStmt does not handle them
	// here; types containing resources route through resource-aware
	// emit code in the helper body.
	if _, ok := t.(*TypeOwn); ok {
		panic("witgen: elemMemReadStmt called on TypeOwn — caller must dispatch resource handling")
	}
	if _, ok := t.(*TypeBorrow); ok {
		panic("witgen: elemMemReadStmt called on TypeBorrow — caller must dispatch resource handling")
	}
	panic(fmt.Sprintf("witgen: elemMemReadStmt: unhandled %T", t))
}

// elemMemWriteStmtErr is the write counterpart of elemMemReadStmt.
// Same caveats: compounds are routed by the caller; resources are
// dispatched by family-aware emit code.
func elemMemWriteStmtErr(t Type, ptrExpr, valExpr, memExpr, helperName, op string, sink errSink) string {
	if p, ok := t.(Prim); ok {
		return primMemWriteStmtErr(p, ptrExpr, valExpr, memExpr, helperName, op, sink)
	}
	if e, ok := t.(*TypeEnum); ok {
		errExpr := fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: %s: bad memory write")`, helperName, op)
		_ = e
		return fmt.Sprintf(`if ok_ := %s.WriteByte(%s, uint8(%s)); !ok_ {
%s
}`, memExpr, ptrExpr, valExpr, sink.Emit(errExpr))
	}
	if f, ok := t.(*TypeFlags); ok {
		errExpr := fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: %s: bad memory write")`, helperName, op)
		_ = f
		return fmt.Sprintf(`if ok_ := %s.WriteUint32Le(%s, uint32(%s)); !ok_ {
%s
}`, memExpr, ptrExpr, valExpr, sink.Emit(errExpr))
	}
	if _, ok := t.(*TypeOwn); ok {
		panic("witgen: elemMemWriteStmtErr called on TypeOwn — caller must dispatch resource handling")
	}
	if _, ok := t.(*TypeBorrow); ok {
		panic("witgen: elemMemWriteStmtErr called on TypeBorrow — caller must dispatch resource handling")
	}
	panic(fmt.Sprintf("witgen: elemMemWriteStmtErr: unhandled %T", t))
}

// liftFlatEnumExpr returns a Go expression that decodes an enum value
// from the flat-stack slot expression src.
func liftFlatEnumExpr(e *TypeEnum, src string) string {
	return fmt.Sprintf("%s(%s)", GoTypeOf(e), src)
}

// liftFlatFlagsExpr returns a Go expression that decodes a flags value
// from the flat-stack slot expression src.
func liftFlatFlagsExpr(f *TypeFlags, src string) string {
	return fmt.Sprintf("%s(%s)", GoTypeOf(f), src)
}

// lowerFlatEnumExpr returns a Go expression that encodes an enum value
// into a uint64 flat slot.
func lowerFlatEnumExpr(e *TypeEnum, valExpr string) string {
	return fmt.Sprintf("uint64(%s)", valExpr)
}

// lowerFlatFlagsExpr returns a Go expression that encodes a flags value
// into a uint64 flat slot.
func lowerFlatFlagsExpr(f *TypeFlags, valExpr string) string {
	return fmt.Sprintf("uint64(%s)", valExpr)
}

// recordFieldTypes returns the slice of field Types of a record, used
// for ABI offset/alignment computation that expects a []Type input.
func recordFieldTypes(rec *TypeRecord) []Type {
	out := make([]Type, len(rec.Fields))
	for i, f := range rec.Fields {
		out[i] = f.Type
	}
	return out
}

// emitMode selects which of the eight helper bodies to emit.
type emitMode int

const (
	// Trampoline-side family — to/fromGo. cc-only signature; no
	// canon-table handoff.
	modeToGoFlat emitMode = iota
	modeToGoMem
	modeFromGoFlat
	modeFromGoMem
	// Wrap-side family — lift/lower. Takes both caller and callee
	// CallContexts; performs cross-table transfer for resources.
	modeLiftFlat
	modeLiftMem
	modeLowerFlat
	modeLowerMem
)

// helperBody produces the Go statements that form the body of a helper
// function for type t in the given mode. Output is raw — go/format
// normalises whitespace later.
func helperBody(t Type, mode emitMode) string {
	var w strings.Builder
	switch mode {
	case modeToGoFlat:
		emitToGoFlat(&w, t)
	case modeToGoMem:
		emitToGoMem(&w, t)
	case modeFromGoFlat:
		emitFromGoFlat(&w, t)
	case modeFromGoMem:
		emitFromGoMem(&w, t)
	case modeLiftFlat:
		emitLiftFlat(&w, t)
	case modeLiftMem:
		emitLiftMem(&w, t)
	case modeLowerFlat:
		emitLowerFlat(&w, t)
	case modeLowerMem:
		emitLowerMem(&w, t)
	default:
		panic("witgen: helperBody: unknown mode")
	}
	return w.String()
}

// emitLiftFlat writes the body of liftFlat<Name>(ctx, caller, callee, h, ty, stack []uint64) (T, error).
// Wire bytes (memory + flat slots) live on the callee side. Resource
// handles handed out cross-table (callee.ReleaseOwn → caller.IssueOwn).
//
// ty is the runtime wacogo.Type for the value being lifted, sourced
// from the wrapped function's signature. Each compound branch casts ty
// to the appropriate runtime structural type and threads the child
// type-expressions into nested helper calls so resource leaves can
// recover their per-instance TR without consulting any package-local
// slot index.
func emitLiftFlat(w *strings.Builder, t Type) {
	helperName := "liftFlat" + TypeName(t)
	sink := errSinkLift{ZeroValueExpr: ZeroValueExpr(t)}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "return %s, nil\n", liftFlatExpr(v, "stack[0]"))
	case TypeString:
		fmt.Fprintf(w, `ptr_ := uint32(stack[0])
ln_ := uint32(stack[1])
bs_, ok_ := callee.Memory().Read(ptr_, ln_)
if !ok_ {
%s
}
return string(bs_), nil
`, sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory read")`, helperName)))
	case *TypeList:
		elemTy := GoTypeOf(v.Elem)
		elemSize := Size(v.Elem)
		fmt.Fprintf(w, "lstT_ := ty.(wacogo.TypeList)\n")
		fmt.Fprintf(w, "_ = lstT_\n")
		fmt.Fprintf(w, "ptr_ := uint32(stack[0])\n")
		fmt.Fprintf(w, "ln_ := uint32(stack[1])\n")
		fmt.Fprintf(w, "out_ := make([]%s, ln_)\n", elemTy)
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemReadIntoLift(w, v.Elem, "out_[i_]", fmt.Sprintf("ptr_ + i_*%d", elemSize), helperName, "elem", sink, "\t", "lstT_.Elem")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "return out_, nil\n")
	case *TypeTuple:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "tupT_ := ty.(wacogo.TypeTuple)\n")
		fmt.Fprintf(w, "_ = tupT_\n")
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f)
			dst := fmt.Sprintf("v_.F%d", i)
			emitFlatSlotReadIntoLift(w, f, dst, offset, n, helperName, fmt.Sprintf("field F%d", i), sink, "", fmt.Sprintf("tupT_.Types[%d]", i))
			offset += n
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeRecord:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "recT_ := ty.(wacogo.TypeRecord)\n")
		fmt.Fprintf(w, "_ = recT_\n")
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f.Type)
			dst := fmt.Sprintf("v_.%s", f.GoName)
			emitFlatSlotReadIntoLift(w, f.Type, dst, offset, n, helperName, "field "+f.GoName, sink, "", fmt.Sprintf("recT_.Fields[%d].Type", i))
			offset += n
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeOption:
		emitLiftFlatOption(w, v, helperName, sink)
	case *TypeResult:
		emitLiftFlatResult(w, v, helperName, sink)
	case *TypeVariant:
		emitLiftFlatVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitLiftFlat: unhandled %T", t))
	}
}

// emitLiftMem writes the body of liftMem<Name>(ctx, caller, callee, h, ty, ptr uint32) (T, error).
// Same conventions as emitLiftFlat but reads from callee memory at ptr.
func emitLiftMem(w *strings.Builder, t Type) {
	helperName := "liftMem" + TypeName(t)
	sink := errSinkLift{ZeroValueExpr: ZeroValueExpr(t)}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "var out_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "%s\n", primMemReadStmt(v, "out_", "ptr", "callee.Memory()", helperName, "primitive", sink))
		fmt.Fprintf(w, "return out_, nil\n")
	case TypeString:
		fmt.Fprintf(w, "var sptr_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "sptr_", "ptr", "callee.Memory()", helperName, "string ptr", sink))
		fmt.Fprintf(w, "var ln_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "ln_", "ptr + 4", "callee.Memory()", helperName, "string len", sink))
		fmt.Fprintf(w, `bs_, ok_ := callee.Memory().Read(sptr_, ln_)
if !ok_ {
%s
}
return string(bs_), nil
`, sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory read")`, helperName)))
	case *TypeList:
		elemTy := GoTypeOf(v.Elem)
		elemSize := Size(v.Elem)
		fmt.Fprintf(w, "lstT_ := ty.(wacogo.TypeList)\n")
		fmt.Fprintf(w, "_ = lstT_\n")
		fmt.Fprintf(w, "var lptr_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "lptr_", "ptr", "callee.Memory()", helperName, "list ptr", sink))
		fmt.Fprintf(w, "var ln_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "ln_", "ptr + 4", "callee.Memory()", helperName, "list len", sink))
		fmt.Fprintf(w, "out_ := make([]%s, ln_)\n", elemTy)
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemReadIntoLift(w, v.Elem, "out_[i_]", fmt.Sprintf("lptr_ + i_*%d", elemSize), helperName, "list elem", sink, "\t", "lstT_.Elem")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "return out_, nil\n")
	case *TypeTuple:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "tupT_ := ty.(wacogo.TypeTuple)\n")
		fmt.Fprintf(w, "_ = tupT_\n")
		for i, f := range v.Fields {
			off := FieldOffset(v.Fields, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemReadIntoLift(w, f, fmt.Sprintf("v_.F%d", i), ptrExpr, helperName, fmt.Sprintf("field F%d", i), sink, "", fmt.Sprintf("tupT_.Types[%d]", i))
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeRecord:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "recT_ := ty.(wacogo.TypeRecord)\n")
		fmt.Fprintf(w, "_ = recT_\n")
		fieldTys := recordFieldTypes(v)
		for i, f := range v.Fields {
			off := FieldOffset(fieldTys, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemReadIntoLift(w, f.Type, fmt.Sprintf("v_.%s", f.GoName), ptrExpr, helperName, "field "+f.GoName, sink, "", fmt.Sprintf("recT_.Fields[%d].Type", i))
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeOption:
		emitLiftMemOption(w, v, helperName, sink)
	case *TypeResult:
		emitLiftMemResult(w, v, helperName, sink)
	case *TypeVariant:
		emitLiftMemVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitLiftMem: unhandled %T", t))
	}
}

// emitLowerFlat writes the body of lowerFlat<Name>(ctx, caller, callee, h, ty, stack []uint64, v T) error.
// Wire bytes (memory + flat slots) live on the callee side. Resource
// handles handed in cross-table (caller.ReleaseOwn → callee.IssueOwn).
// See emitLiftFlat for ty/tyExpr semantics.
func emitLowerFlat(w *strings.Builder, t Type) {
	helperName := "lowerFlat" + TypeName(t)
	sink := errSinkLower{}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "stack[0] = %s\n", lowerFlatExpr(v, "v"))
	case TypeString:
		fmt.Fprintf(w, `bs_ := []byte(v)
ptr_, err_ := callee.Realloc(ctx, 0, 0, 1, uint32(len(bs_)))
if err_ != nil {
%s
}
if ok_ := callee.Memory().Write(ptr_, bs_); !ok_ {
%s
}
stack[0] = uint64(ptr_)
stack[1] = uint64(len(bs_))
`,
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)),
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory write")`, helperName)),
		)
	case *TypeList:
		elemSize := Size(v.Elem)
		elemAlign := Align(v.Elem)
		fmt.Fprintf(w, "lstT_ := ty.(wacogo.TypeList)\n")
		fmt.Fprintf(w, "_ = lstT_\n")
		fmt.Fprintf(w, "ln_ := uint32(len(v))\n")
		fmt.Fprintf(w, "ptr_, err_ := callee.Realloc(ctx, 0, 0, %d, ln_*%d)\n", elemAlign, elemSize)
		fmt.Fprintf(w, "if err_ != nil {\n%s\n}\n",
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)))
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemWriteFromLower(w, v.Elem, "v[i_]", fmt.Sprintf("ptr_ + i_*%d", elemSize), helperName, "list elem", sink, "\t", "lstT_.Elem")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "stack[0] = uint64(ptr_)\n")
		fmt.Fprintf(w, "stack[1] = uint64(ln_)\n")
	case *TypeTuple:
		fmt.Fprintf(w, "tupT_ := ty.(wacogo.TypeTuple)\n")
		fmt.Fprintf(w, "_ = tupT_\n")
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f)
			emitFlatSlotWriteFromLower(w, f, fmt.Sprintf("v.F%d", i), offset, n, helperName, fmt.Sprintf("field F%d", i), sink, "", fmt.Sprintf("tupT_.Types[%d]", i))
			offset += n
		}
	case *TypeRecord:
		fmt.Fprintf(w, "recT_ := ty.(wacogo.TypeRecord)\n")
		fmt.Fprintf(w, "_ = recT_\n")
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f.Type)
			emitFlatSlotWriteFromLower(w, f.Type, fmt.Sprintf("v.%s", f.GoName), offset, n, helperName, "field "+f.GoName, sink, "", fmt.Sprintf("recT_.Fields[%d].Type", i))
			offset += n
		}
	case *TypeOption:
		emitLowerFlatOption(w, v, helperName, sink)
	case *TypeResult:
		emitLowerFlatResult(w, v, helperName, sink)
	case *TypeVariant:
		emitLowerFlatVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitLowerFlat: unhandled %T", t))
	}
}

// emitLowerMem writes the body of lowerMem<Name>(ctx, caller, callee, h, ty, ptr uint32, v T) error.
// Same conventions as emitLowerFlat but writes to callee memory at ptr.
func emitLowerMem(w *strings.Builder, t Type) {
	helperName := "lowerMem" + TypeName(t)
	sink := errSinkLower{}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "%s\n", primMemWriteStmtErr(v, "ptr", "v", "callee.Memory()", helperName, "primitive", sink))
	case TypeString:
		fmt.Fprintf(w, `bs_ := []byte(v)
sptr_, err_ := callee.Realloc(ctx, 0, 0, 1, uint32(len(bs_)))
if err_ != nil {
%s
}
if ok_ := callee.Memory().Write(sptr_, bs_); !ok_ {
%s
}
%s
%s
`,
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)),
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory write")`, helperName)),
			primMemWriteStmtErr(PrimU32, "ptr", "sptr_", "callee.Memory()", helperName, "string ptr", sink),
			primMemWriteStmtErr(PrimU32, "ptr + 4", "uint32(len(bs_))", "callee.Memory()", helperName, "string len", sink),
		)
	case *TypeList:
		elemSize := Size(v.Elem)
		elemAlign := Align(v.Elem)
		fmt.Fprintf(w, "lstT_ := ty.(wacogo.TypeList)\n")
		fmt.Fprintf(w, "_ = lstT_\n")
		fmt.Fprintf(w, "ln_ := uint32(len(v))\n")
		fmt.Fprintf(w, "lptr_, err_ := callee.Realloc(ctx, 0, 0, %d, ln_*%d)\n", elemAlign, elemSize)
		fmt.Fprintf(w, "if err_ != nil {\n%s\n}\n",
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)))
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemWriteFromLower(w, v.Elem, "v[i_]", fmt.Sprintf("lptr_ + i_*%d", elemSize), helperName, "list elem", sink, "\t", "lstT_.Elem")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "%s\n", primMemWriteStmtErr(PrimU32, "ptr", "lptr_", "callee.Memory()", helperName, "list ptr", sink))
		fmt.Fprintf(w, "%s\n", primMemWriteStmtErr(PrimU32, "ptr + 4", "ln_", "callee.Memory()", helperName, "list len", sink))
	case *TypeTuple:
		fmt.Fprintf(w, "tupT_ := ty.(wacogo.TypeTuple)\n")
		fmt.Fprintf(w, "_ = tupT_\n")
		for i, f := range v.Fields {
			off := FieldOffset(v.Fields, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemWriteFromLower(w, f, fmt.Sprintf("v.F%d", i), ptrExpr, helperName, fmt.Sprintf("field F%d", i), sink, "", fmt.Sprintf("tupT_.Types[%d]", i))
		}
	case *TypeRecord:
		fmt.Fprintf(w, "recT_ := ty.(wacogo.TypeRecord)\n")
		fmt.Fprintf(w, "_ = recT_\n")
		fieldTys := recordFieldTypes(v)
		for i, f := range v.Fields {
			off := FieldOffset(fieldTys, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemWriteFromLower(w, f.Type, fmt.Sprintf("v.%s", f.GoName), ptrExpr, helperName, "field "+f.GoName, sink, "", fmt.Sprintf("recT_.Fields[%d].Type", i))
		}
	case *TypeOption:
		emitLowerMemOption(w, v, helperName, sink)
	case *TypeResult:
		emitLowerMemResult(w, v, helperName, sink)
	case *TypeVariant:
		emitLowerMemVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitLowerMem: unhandled %T", t))
	}
}

// Functions below emit per-type helper bodies for the eight-mode
// family. Trampoline-side bodies (emitToGoFlat/Mem, emitFromGoFlat/Mem)
// land further down in this file. Wrap-side bodies (emitLiftFlat/Mem,
// emitLowerFlat/Mem) immediately below take (caller, callee, h) and
// perform cross-table handoff for resource-bearing compounds. The
// emit produces sealed-interface Result values (Result type emission
// uses this sealed-interface shape).

// emitHelper writes the full Go function declaration (signature + body)
// for type t in the given mode. The two parallel families differ in
// signature; the body is produced by helperBody.
func emitHelper(w *strings.Builder, t Type, mode emitMode) {
	name := TypeName(t)
	goTy := GoTypeOf(t)
	fmt.Fprint(w, emitHelperSig(name, goTy, mode))
	fmt.Fprint(w, " {\n")
	fmt.Fprint(w, helperBody(t, mode))
	if isLowerMode(mode) {
		fmt.Fprint(w, "\treturn nil\n")
	}
	fmt.Fprint(w, "}\n\n")
}

func isLowerMode(m emitMode) bool {
	return m == modeFromGoFlat || m == modeFromGoMem || m == modeLowerFlat || m == modeLowerMem
}

// emitHelperSig returns the full helper function signature line (without
// the trailing "{").
func emitHelperSig(name, goTy string, mode emitMode) string {
	switch mode {
	case modeToGoFlat:
		return fmt.Sprintf("func toGoFlat%s(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, stack []uint64) (%s, error)", name, goTy)
	case modeToGoMem:
		return fmt.Sprintf("func toGoMem%s(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, ptr uint32) (%s, error)", name, goTy)
	case modeFromGoFlat:
		return fmt.Sprintf("func fromGoFlat%s(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64, v %s) error", name, goTy)
	case modeFromGoMem:
		return fmt.Sprintf("func fromGoMem%s(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, ptr uint32, v %s) error", name, goTy)
	case modeLiftFlat:
		return fmt.Sprintf("func liftFlat%s(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, stack []uint64) (%s, error)", name, goTy)
	case modeLiftMem:
		return fmt.Sprintf("func liftMem%s(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, ptr uint32) (%s, error)", name, goTy)
	case modeLowerFlat:
		return fmt.Sprintf("func lowerFlat%s(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, stack []uint64, v %s) error", name, goTy)
	case modeLowerMem:
		return fmt.Sprintf("func lowerMem%s(ctx context.Context, caller, callee *host.CallContext, h *host.ComponentInstance, ty wacogo.Type, ptr uint32, v %s) error", name, goTy)
	}
	panic("witgen: emitHelperSig: unknown mode")
}

// emitWrapFunc writes the wrap<Method>(f *Factory) host.Func definition.
func emitWrapFunc(w *strings.Builder, fn *Func) {
	fmt.Fprintf(w, "func wrap%s(f *Factory) host.Func {\n", fn.GoName)
	fmt.Fprint(w, "\treturn func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64) error {\n")
	fmt.Fprint(w, "\t\tstate_ := h.UserState().(*instanceState)\n")
	emitTrampolineFnTypeLookup(w, fn.WitName)

	// Lift parameters from stack at known offsets.
	paramOffset := 0
	var argList []string
	for i, p := range fn.Params {
		tyExpr := fmt.Sprintf("fnType_.Params[%d].Type", i)
		n, varName := emitTrampolineParamLift(w, p, paramOffset, false /* selfShortcut */, tyExpr)
		paramOffset += n
		argList = append(argList, varName)
	}

	args := strings.Join(argList, ", ")
	if args == "" {
		args = "ctx"
	} else {
		args = "ctx, " + args
	}
	callExpr := fmt.Sprintf("state_.impl.%s(%s)", fn.GoName, args)
	emitTrampolineUserCall(w, fn.WitName, fn.Result, callExpr)

	fmt.Fprint(w, "\t\treturn nil\n")
	fmt.Fprint(w, "\t}\n")
	fmt.Fprint(w, "}\n\n")
}

// emitTrampolineFnTypeLookup writes the per-call fnType_ lookup that
// resource-typed param lift and toGoX helper calls thread their per-
// instance type identity through. The export name baked in here is the
// canonical wit name (e.g. "greet" or "[method]counter.current"); the
// trampoline runs on the host (defining) side, so cc.Instance() is the
// instance that exports this func, and the per-instance core.FuncType
// is the source of truth for resource-leaf TR identity.
func emitTrampolineFnTypeLookup(w *strings.Builder, witName string) {
	fmt.Fprintf(w, "\t\tfnType_ := cc.Instance().ExportedFunc(%q).Type()\n", witName)
	fmt.Fprint(w, "\t\t_ = fnType_\n")
}

// emitTrampolineUserCall emits the user-impl call plus error propagation
// and result lowering. Every user method returns either `error` (void)
// or `(T, error)`. The trampoline propagates the error as a wasm trap
// (wrapped for context) and lowers the value otherwise.
//
// witName is used to label the wrapping error message; callExpr is the
// fully formed Go call expression (e.g. "state_.impl.Foo(ctx, a, b)" or
// "self.Increment(ctx)").
func emitTrampolineUserCall(w *strings.Builder, witName string, result Type, callExpr string) {
	if result == nil {
		fmt.Fprintf(w, "\t\tif err_ := %s; err_ != nil { return fmt.Errorf(\"wacogo/witgen: %s: impl returned error: %%w\", err_) }\n", callExpr, witName)
		return
	}
	fmt.Fprintf(w, "\t\tresult_, implErr_ := %s\n", callExpr)
	fmt.Fprintf(w, "\t\tif implErr_ != nil { return fmt.Errorf(\"wacogo/witgen: %s: impl returned error: %%w\", implErr_) }\n", witName)
	emitWrapLowerResultGeneric(w, witName, result, "result_", "rerr_")
}

// emitTrampolineParamLift emits the lift code for a single trampoline
// param at flat-stack offset off. Returns the number of slots consumed
// and the Go variable name used to hold the lifted value (which may
// differ from p.GoName when p.GoName would shadow an imported package
// name).
//
// tyExpr is a Go expression evaluating at runtime to the wacogo.Type for
// p.Type; resource-leaf TR identity is resolved through it
// (tyExpr.(wacogo.TypeOwn).ResourceType etc.) instead of the
// componentResourceTypeID slot table, mirroring the wrap-side liftX/lowerX
// path.
//
// For resource-typed params: stack[off] is the canon handle on
// cc.Instance(). The handle is looked up via cc.LookupBorrowable, then
// wrapped via New<R>HandleFrom with either the impl recovered from the
// defining instance's extern table (local R) or a forwarder backed by
// instanceState.<FwdFnsField> (foreign R).
func emitTrampolineParamLift(w *strings.Builder, p *Param, off int, _ bool, tyExpr string) (int, string) {
	n := FlatSlots(p.Type)
	slotExpr := fmt.Sprintf("stack[%d]", off)
	varName := p.GoName
	switch ty := p.Type.(type) {
	case Prim:
		fmt.Fprintf(w, "\t\t%s := %s\n", varName, liftFlatExpr(ty, slotExpr))
	case *TypeEnum:
		fmt.Fprintf(w, "\t\t%s := %s\n", varName, liftFlatEnumExpr(ty, slotExpr))
	case *TypeFlags:
		fmt.Fprintf(w, "\t\t%s := %s\n", varName, liftFlatFlagsExpr(ty, slotExpr))
	case *TypeOwn:
		varName = emitResourceParamLift(w, p.GoName, ty.Resource, slotExpr, false /* isBorrow */, tyExpr)
	case *TypeBorrow:
		varName = emitResourceParamLift(w, p.GoName, ty.Resource, slotExpr, true /* isBorrow */, tyExpr)
	default:
		fmt.Fprintf(w, "\t\tvar %s %s\n", varName, GoTypeOf(p.Type))
		fmt.Fprint(w, "\t\t{\n")
		fmt.Fprintf(w, "\t\t\tv_, err_ := toGoFlat%s(ctx, cc, h, %s, stack[%d:%d])\n", TypeName(p.Type), tyExpr, off, off+n)
		fmt.Fprint(w, "\t\t\tif err_ != nil { return err_ }\n")
		fmt.Fprintf(w, "\t\t\t%s = v_\n", varName)
		fmt.Fprint(w, "\t\t}\n")
	}
	return n, varName
}

// emitResourceParamLift writes the lift body for one resource-typed
// trampoline param, branching on the (own vs borrow) × (local vs
// foreign) matrix:
//   - foreign (own or borrow): wire form is a canon handle in cc's
//     table; look it up and wrap it in a forwarder.
//   - local own: wire form is a canon handle in cc's table.
//   - local borrow: same-component shortcut applies (callee == defining);
//     the wire value is the rep itself, not a handle index. Resolve via
//     h's extern table and wrap in a shortcut-borrow state.
//
// Each emitted block is wrapped in a Go block scope so the locally
// declared helpers (oh_, rep, impl) cannot collide with later params.
//
// Returns the Go variable name used to hold the lifted value (may differ
// from goName when goName would shadow an imported package identifier).
func emitResourceParamLift(w *strings.Builder, goName string, rt *TypeResource, slotExpr string, isBorrow bool, tyExpr string) string {
	// Compute the outer variable name. If goName is the same as the import
	// package qualifier the outer var would shadow the import inside the block,
	// making qualified calls (e.g. network.NewRemote…) fail. Avoid by appending
	// "Param" to the variable name; the argList in the caller uses this name.
	varName := goName
	if rt.IsImported() && goName == rt.GoPackageQualifier {
		varName = goName + "Param"
	}

	tyAssertion := "wacogo.TypeOwn"
	if isBorrow {
		tyAssertion = "wacogo.TypeBorrow"
	}

	if rt.IsImported() {
		// Foreign R: build a forwarder backed by precomputed fns on
		// instanceState and wrap the canon handle in a *<pkg>.<R>Handle
		// via New<R>HandleFrom.
		fmt.Fprintf(w, "\t\tvar %s *%s.%sHandle\n", varName, rt.GoPackageQualifier, rt.WrapName)
		fmt.Fprint(w, "\t\t{\n")
		fmt.Fprintf(w, "\t\t\toh_ := uint32(%s)\n", slotExpr)
		fmt.Fprintf(w, "\t\t\ttr_ := %s.(%s).ResourceType\n", tyExpr, tyAssertion)
		fmt.Fprint(w, "\t\t\trh_, lookupErr_ := cc.LookupBorrowable(tr_, oh_)\n")
		fmt.Fprint(w, "\t\t\tif lookupErr_ != nil { return lookupErr_ }\n")
		fmt.Fprintf(w, "\t\t\tfwd_ := &%s{caller: h, rh: rh_, fns: state_.%s}\n",
			rt.GoPackageQualifier+rt.WrapName+"Forwarder",
			rt.GoPackageQualifier+rt.WrapName+"FwdFns",
		)
		fmt.Fprintf(w, "\t\t\t%s = %s.New%sHandleFrom(rh_, fwd_)\n", varName, rt.GoPackageQualifier, rt.WrapName)
		fmt.Fprint(w, "\t\t}\n")
		return varName
	}
	if isBorrow {
		// Local borrow: same-component shortcut. Wire value is the rep;
		// resolve impl via h's extern table and construct the handle in
		// shortcutBorrowHandleState so LocalImpl returns the impl directly.
		fmt.Fprintf(w, "\t\tvar %s *%sHandle\n", goName, rt.WrapName)
		fmt.Fprint(w, "\t\t{\n")
		fmt.Fprintf(w, "\t\t\toh_ := uint32(%s)\n", slotExpr)
		fmt.Fprint(w, "\t\t\tobj_, ok_ := h.LookupResource(host.ExternHandle(oh_))\n")
		fmt.Fprintf(w, "\t\t\tif !ok_ { return fmt.Errorf(\"wacogo/witgen: borrow<%s>: unknown rep %%d\", oh_) }\n", rt.Name)
		fmt.Fprintf(w, "\t\t\timpl_, _ := obj_.(%s)\n", rt.WrapName)
		fmt.Fprintf(w, "\t\t\t%s = &%sHandle{\n", goName, rt.WrapName)
		fmt.Fprintf(w, "\t\t\t\t%s: impl_,\n", rt.WrapName)
		fmt.Fprint(w, "\t\t\t\tstate: &shortcutBorrowHandleState{defining: h, rep: oh_, impl: obj_},\n")
		fmt.Fprint(w, "\t\t\t}\n")
		fmt.Fprint(w, "\t\t}\n")
		return goName
	}
	// Locally-defined own: wire form is a canon handle in cc's table;
	// look up the live core.ResourceHandle, resolve impl via the defining
	// instance's extern table, and wrap with New<R>HandleFrom.
	fmt.Fprintf(w, "\t\tvar %s *%sHandle\n", goName, rt.WrapName)
	fmt.Fprint(w, "\t\t{\n")
	fmt.Fprintf(w, "\t\t\toh_ := uint32(%s)\n", slotExpr)
	fmt.Fprintf(w, "\t\t\ttr_ := %s.(wacogo.TypeOwn).ResourceType\n", tyExpr)
	fmt.Fprint(w, "\t\t\trh_, lookupErr_ := cc.LookupBorrowable(tr_, oh_)\n")
	fmt.Fprint(w, "\t\t\tif lookupErr_ != nil { return lookupErr_ }\n")
	fmt.Fprintf(w, "\t\t\tvar impl_ %s\n", rt.WrapName)
	fmt.Fprint(w, "\t\t\tif obj_, ok_ := rh_.Type().Instance().LookupExtern(rh_.Rep()); ok_ {\n")
	fmt.Fprintf(w, "\t\t\t\tif typed_, ok2_ := obj_.(%s); ok2_ { impl_ = typed_ }\n", rt.WrapName)
	fmt.Fprint(w, "\t\t\t}\n")
	fmt.Fprintf(w, "\t\t\t%s = New%sHandleFrom(rh_, impl_)\n", goName, rt.WrapName)
	fmt.Fprint(w, "\t\t}\n")
	return goName
}

// emitWrapLowerResultGeneric is the call-style lower-result tail of
// wrap_*: either flat-mode single-slot inline cast, or mem-mode
// realloc+lowerMem with the pointer placed in stack[0]. errVar is the
// local name for the realloc error variable — pass a fresh name when
// an outer `err` is already in scope.
func emitWrapLowerResultGeneric(w *strings.Builder, witName string, t Type, valExpr, errVar string) {
	if FlatSlots(t) > 1 {
		sz := Size(t)
		al := Align(t)
		fmt.Fprintf(w, "\t\toutPtr_, %s := cc.Realloc(ctx, 0, 0, %d, %d)\n", errVar, al, sz)
		fmt.Fprintf(w, "\t\tif %s != nil { return fmt.Errorf(\"wacogo/witgen: %s: realloc failed: %%w\", %s) }\n", errVar, witName, errVar)
		fmt.Fprintf(w, "\t\tif err_ := fromGoMem%s(ctx, cc, h, outPtr_, %s); err_ != nil { return err_ }\n", TypeName(t), valExpr)
		fmt.Fprint(w, "\t\tstack[0] = uint64(outPtr_)\n")
		return
	}
	switch ty := t.(type) {
	case Prim:
		fmt.Fprintf(w, "\t\tstack[0] = %s\n", lowerFlatExpr(ty, valExpr))
	case *TypeEnum:
		fmt.Fprintf(w, "\t\tstack[0] = %s\n", lowerFlatEnumExpr(ty, valExpr))
	case *TypeFlags:
		fmt.Fprintf(w, "\t\tstack[0] = %s\n", lowerFlatFlagsExpr(ty, valExpr))
	case *TypeOwn:
		emitOwnResultLower(w, ty.Resource, valExpr)
	default:
		fmt.Fprintf(w, "\t\tif err_ := fromGoFlat%s(ctx, cc, h, stack[0:1], %s); err_ != nil { return err_ }\n", TypeName(t), valExpr)
	}
}

// emitOwnResultLower writes the trampoline tail for an own<R> result,
// branching on whether R is locally defined or imported:
//
//   - local R: result.bind(cc, h) drives the *<R>Handle's state machine
//     to register the impl on h's extTable and issue an own canon entry
//     on cc.Instance(). result.Invalidate() then marks the Go handle
//     unusable since ownership has transferred to the wasm caller.
//   - foreign R: result is a *<pkg>.<R>Handle from another package.
//     <pkg>.HandleValueFor drives the same bind state machine across the
//     package boundary; result.Invalidate() likewise transfers ownership.
//     stack[0] receives the canon handle in both cases.
func emitOwnResultLower(w *strings.Builder, rt *TypeResource, valExpr string) {
	fmt.Fprint(w, "\t\t{\n")
	if rt.IsImported() {
		fmt.Fprintf(w, "\t\t\toh_, err_ := %s.HandleValueFor(cc, h, %s)\n", rt.GoPackageQualifier, valExpr)
	} else {
		fmt.Fprintf(w, "\t\t\toh_, err_ := %s.bind(cc, h)\n", valExpr)
	}
	fmt.Fprint(w, "\t\t\tif err_ != nil { return fmt.Errorf(\"wacogo/witgen: own-result bind failed: %w\", err_) }\n")
	fmt.Fprintf(w, "\t\t\t%s.Invalidate()\n", valExpr)
	fmt.Fprint(w, "\t\t\tstack[0] = uint64(oh_)\n")
	fmt.Fprint(w, "\t\t}\n")
}

// emitWrapCtor emits the wrapNew<R>(f *Factory) host.Func definition
// for a resource constructor.
//
// The user impl's Constructor returns *<R>Handle (an unregistered handle
// wrapping the impl in a local dispatch wrapper). result.bind(cc, h)
// drives the state machine to register the impl on h's extTable and
// issue an own canon entry on cc.Instance(); result.Invalidate() then
// transfers ownership of the canon entry to the wasm caller.
func emitWrapCtor(w *strings.Builder, rt *TypeResource) {
	fmt.Fprintf(w, "func wrapNew%s(f *Factory) host.Func {\n", rt.GoName)
	fmt.Fprint(w, "\treturn func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64) error {\n")
	fmt.Fprint(w, "\t\tstate_ := h.UserState().(*instanceState)\n")
	witName := "[constructor]" + rt.Name
	emitTrampolineFnTypeLookup(w, witName)
	off := 0
	var argList []string
	for i, p := range rt.Ctor.Params {
		tyExpr := fmt.Sprintf("fnType_.Params[%d].Type", i)
		n, varName := emitTrampolineParamLift(w, p, off, false, tyExpr)
		off += n
		argList = append(argList, varName)
	}
	args := strings.Join(argList, ", ")
	if args == "" {
		args = "ctx"
	} else {
		args = "ctx, " + args
	}
	fmt.Fprintf(w, "\t\tresult_, err_ := state_.impl.New%s(%s)\n", rt.WrapName, args)
	fmt.Fprintf(w, "\t\tif err_ != nil { return fmt.Errorf(\"wacogo/witgen: %s: impl returned error: %%w\", err_) }\n", witName)
	fmt.Fprint(w, "\t\toh_, err_ := result_.bind(cc, h)\n")
	fmt.Fprintf(w, "\t\tif err_ != nil { return fmt.Errorf(\"wacogo/witgen: %s: bind failed: %%w\", err_) }\n", witName)
	fmt.Fprint(w, "\t\tresult_.Invalidate()\n")
	fmt.Fprint(w, "\t\tstack[0] = uint64(oh_)\n")
	fmt.Fprint(w, "\t\treturn nil\n")
	fmt.Fprint(w, "\t}\n")
	fmt.Fprint(w, "}\n\n")
}

// emitWrapMethod emits the wrap<R><Method>(f *Factory) host.Func
// definition for a resource per-instance method.
//
// The first stack slot is the self: borrow<R> receiver of a locally-
// defined resource — by the design's row-1 "self" sub-case, the
// trampoline takes the same-component shortcut: stack[0] is the rep,
// look up the impl in the extTable, and call the user's Go method
// directly on the impl rather than constructing a *<R>Handle wrapper.
//
// Other (non-self) params lift via emitTrampolineParamLift. State is
// also looked up because foreign-resource params need
// state.<FwdFnsField> to construct the forwarder, and the user impl is
// reached through state_.impl on methods routed back through the
// parent interface (none today, but kept consistent with funcs/statics).
func emitWrapMethod(w *strings.Builder, rt *TypeResource, m ResourceMethod) {
	fmt.Fprintf(w, "func wrap%s%s(f *Factory) host.Func {\n", rt.GoName, m.GoName)
	fmt.Fprint(w, "\treturn func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64) error {\n")
	witName := "[method]" + rt.Name + "." + m.Name
	// state is referenced only by foreign-resource param lifts
	// (state.<imp>RT); for purely local-resource and primitive params
	// the lookup is dead weight, so skip it in that case.
	needsState := false
	for _, p := range m.Params {
		if r, ok := paramResource(p); ok && r.IsImported() {
			needsState = true
			break
		}
	}
	if needsState {
		fmt.Fprint(w, "\t\tstate_ := h.UserState().(*instanceState)\n")
	}
	emitTrampolineFnTypeLookup(w, witName)
	// self — borrow<R>, R local: same-component shortcut. stack[0] is
	// the rep stored in h's extTable; resolve impl directly and call the
	// method on it.
	fmt.Fprintf(w, "\t\tselfObj_, ok_ := h.LookupResource(host.ExternHandle(uint32(stack[0])))\n")
	fmt.Fprintf(w, "\t\tif !ok_ { return fmt.Errorf(\"wacogo/witgen: %s: missing rep\") }\n", witName)
	fmt.Fprintf(w, "\t\tself_ := selfObj_.(%s)\n", rt.WrapName)
	off := 1
	var argList []string
	for i, p := range m.Params {
		tyExpr := fmt.Sprintf("fnType_.Params[%d].Type", i+1)
		n, varName := emitTrampolineParamLift(w, p, off, false, tyExpr)
		off += n
		argList = append(argList, varName)
	}
	args := strings.Join(argList, ", ")
	if args == "" {
		args = "ctx"
	} else {
		args = "ctx, " + args
	}
	callExpr := fmt.Sprintf("self_.%s(%s)", m.GoName, args)
	emitTrampolineUserCall(w, witName, m.Result, callExpr)
	fmt.Fprint(w, "\t\treturn nil\n")
	fmt.Fprint(w, "\t}\n")
	fmt.Fprint(w, "}\n\n")
}

// paramResource returns the resource type referenced by p (own or borrow)
// and true; otherwise nil and false.
func paramResource(p *Param) (*TypeResource, bool) {
	switch ty := p.Type.(type) {
	case *TypeOwn:
		return ty.Resource, true
	case *TypeBorrow:
		return ty.Resource, true
	}
	return nil, false
}

// emitWrapStatic emits the wrap<R><Static>(f *Factory) host.Func
// definition for a resource static function. Statics dispatch through
// state_.impl like freestanding funcs.
func emitWrapStatic(w *strings.Builder, rt *TypeResource, s ResourceStatic) {
	fmt.Fprintf(w, "func wrap%s(f *Factory) host.Func {\n", s.GoName)
	fmt.Fprint(w, "\treturn func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64) error {\n")
	witName := "[static]" + rt.Name + "." + s.Name
	fmt.Fprint(w, "\t\tstate_ := h.UserState().(*instanceState)\n")
	emitTrampolineFnTypeLookup(w, witName)
	off := 0
	var argList []string
	for i, p := range s.Params {
		tyExpr := fmt.Sprintf("fnType_.Params[%d].Type", i)
		n, varName := emitTrampolineParamLift(w, p, off, false, tyExpr)
		off += n
		argList = append(argList, varName)
	}
	args := strings.Join(argList, ", ")
	if args == "" {
		args = "ctx"
	} else {
		args = "ctx, " + args
	}
	callExpr := fmt.Sprintf("state_.impl.%s(%s)", s.GoName, args)
	emitTrampolineUserCall(w, witName, s.Result, callExpr)
	fmt.Fprint(w, "\t\treturn nil\n")
	fmt.Fprint(w, "\t}\n")
	fmt.Fprint(w, "}\n\n")
}

// emitBindFile produces the full text of <iface>.bind.go for one Interface.
func emitBindFile(iface *Interface, sourceFile string) ([]byte, error) {
	var buf bytes.Buffer

	headerView := bindView{
		Interface:  iface,
		SourceFile: sourceFile,
	}
	header, err := renderTemplate("file_header.tmpl", headerView)
	if err != nil {
		return nil, err
	}
	buf.Write(header)
	buf.WriteString("\n")

	// Superset import block; formatGoSource drops what isn't referenced.
	buf.WriteString("import (\n")
	buf.WriteString("\t\"context\"\n")
	buf.WriteString("\t\"fmt\"\n\n")
	buf.WriteString("\t\"github.com/partite-ai/wacogo\"\n")
	buf.WriteString("\t\"github.com/partite-ai/wacogo/host\"\n")
	for _, imp := range iface.Imports {
		if imp.GoPackagePath != "" {
			fmt.Fprintf(&buf, "\t%s %q\n", imp.GoPackage, imp.GoPackagePath)
		}
	}
	buf.WriteString(")\n\n")

	body, err := renderTemplate("factory.tmpl", iface)
	if err != nil {
		return nil, err
	}
	buf.Write(body)
	buf.WriteString("\n")

	// Emit per-type lift/lower helpers (one quartet per non-primitive type).
	types := CollectTypes(iface)
	helpers := HelperTypes(types)
	var hb strings.Builder
	for _, t := range helpers {
		emitHelper(&hb, t, modeToGoFlat)
		emitHelper(&hb, t, modeToGoMem)
		emitHelper(&hb, t, modeFromGoFlat)
		emitHelper(&hb, t, modeFromGoMem)
		emitHelper(&hb, t, modeLiftFlat)
		emitHelper(&hb, t, modeLiftMem)
		emitHelper(&hb, t, modeLowerFlat)
		emitHelper(&hb, t, modeLowerMem)
	}
	buf.WriteString(hb.String())

	// Emit wrap_<Method> for each function.
	var wb strings.Builder
	for _, f := range iface.Funcs {
		emitWrapFunc(&wb, f)
	}
	// Emit wrap_New<R> / wrap_<R><Method> / wrap_<R><Static> per resource.
	for _, rt := range iface.Resources {
		if rt.Ctor != nil {
			emitWrapCtor(&wb, rt)
		}
		for _, m := range rt.Methods {
			emitWrapMethod(&wb, rt, m)
		}
		for _, s := range rt.Statics {
			emitWrapStatic(&wb, rt, s)
		}
	}
	// Emit per-imported-resource forwarder + fns types and methods.
	// Reserved is the set of imported package qualifiers that must not
	// be used as parameter names in forwarder method bodies.
	reserved := make(map[string]bool)
	for _, imp := range iface.Imports {
		if imp.GoPackage != "" {
			reserved[imp.GoPackage] = true
		}
	}
	for _, imp := range iface.Imports {
		for _, ir := range imp.Resources {
			emitImportedResourceForwarder(&wb, imp, ir, reserved)
		}
	}
	buf.WriteString(wb.String())

	return formatGoSource(iface.GoPackage+".bind.go", buf.Bytes())
}

// emitImportedResourceForwarder emits a per-method *host.ExportedFunc
// fns table type, a forwarder type that satisfies the foreign <R>
// interface, and one method per resource method on the forwarder.
// The fns table itself is precomputed once per importer instance at
// NewInstance time and held on instanceState.<FwdFnsField>.
func emitImportedResourceForwarder(w *strings.Builder, imp ImportRef, ir ImportedResource, reserved map[string]bool) {
	if ir.Source == nil {
		return
	}
	methods := ir.QualifiedMethods
	if methods == nil {
		methods = ir.Source.Methods
	}
	fmt.Fprintf(w, "// %s holds precomputed *host.ExportedFunc per method of %s.%s,\n", ir.FwdFnsType, imp.GoPackage, ir.WrapName)
	fmt.Fprintf(w, "// resolved against the lender instance at NewInstance time. Shared by\n")
	fmt.Fprintf(w, "// every %s constructed under this importer instance.\n", ir.ForwarderType)
	fmt.Fprintf(w, "type %s struct {\n", ir.FwdFnsType)
	for _, m := range methods {
		fmt.Fprintf(w, "\tfn%s *host.ExportedFunc\n", m.GoName)
	}
	fmt.Fprint(w, "}\n\n")

	fmt.Fprintf(w, "// %s satisfies %s.%s by forwarding each method to the lender\n", ir.ForwarderType, imp.GoPackage, ir.WrapName)
	fmt.Fprintf(w, "// instance via the precomputed fns table.\n")
	fmt.Fprintf(w, "type %s struct {\n", ir.ForwarderType)
	fmt.Fprint(w, "\tcaller *host.ComponentInstance\n")
	fmt.Fprint(w, "\trh     wacogo.ResourceHandle\n")
	fmt.Fprintf(w, "\tfns    *%s\n", ir.FwdFnsType)
	fmt.Fprint(w, "}\n\n")

	for _, m := range methods {
		emitForwarderMethod(w, ir, m, reserved)
	}
}

// emitForwarderMethod emits one method body on *<Forwarder> for an
// instance method of the imported resource. The method body mirrors
// the same lift/lower closure shape as wrap.go method bodies, but
// dispatches via the precomputed f.fns.fn<M> against the lender
// instance using f.caller as the host wrapper. The hidden self
// borrow<R> argument is encoded as f.rh.HandleID() in stack[0].
func emitForwarderMethod(w *strings.Builder, ir ImportedResource, m ResourceMethod, reserved map[string]bool) {
	sig := buildMethodSignature(m.GoName, m.Params, m.Result, reserved)
	fmt.Fprintf(w, "func (f *%s) %s {\n", ir.ForwarderType, sig)
	emitForwarderBody(w, ir, m, reserved)
	fmt.Fprint(w, "}\n\n")
}

// emitForwarderBody emits the body of one forwarder method. Structure:
// declare result vars, build CallRaw write+read closures, capture errs,
// return.
func emitForwarderBody(w *strings.Builder, ir ImportedResource, m ResourceMethod, reserved map[string]bool) {
	if m.Result != nil {
		fmt.Fprintf(w, "\tvar resVal_ %s\n", goTypeForSide(m.Result, false))
	}
	needsResErr := bodyWritesResErr(m.Params, m.Result)
	if needsResErr {
		fmt.Fprint(w, "\tvar resErr_ error\n")
	}
	for i, p := range m.Params {
		paramName := p.GoName
		if reserved[paramName] {
			paramName = "p_" + paramName
		}
		fmt.Fprintf(w, "\targ%d_ := %s\n", i, paramName)
	}
	fmt.Fprint(w, "\t{\n")
	fmt.Fprintf(w, "\t\tcallErr_ := f.fns.fn%s.CallRaw(ctx, f.caller,\n", m.GoName)
	// write closure: self + params
	fmt.Fprint(w, "\t\t\tfunc(caller, callee *host.CallContext, stack []uint64) {\n")
	fmt.Fprint(w, "\t\t\t\tstack[0] = uint64(f.rh.HandleID())\n")
	off := 1
	fnExpr := "f.fns.fn" + m.GoName
	for i, p := range m.Params {
		// Implicit-self borrow occupies WIT param index 0; user params start at 1.
		emitWrapArgWrite(w, p, fmt.Sprintf("arg%d_", i), off, "\t\t\t\t", "f.caller", fnExpr, 1+i)
		off += FlatSlots(p.Type)
	}
	fmt.Fprint(w, "\t\t\t},\n")
	// read closure: result lift
	fmt.Fprint(w, "\t\t\tfunc(caller, callee *host.CallContext, stack []uint64) {\n")
	if m.Result != nil {
		emitWrapResultRead(w, m.Result, "f", "f.caller", fnExpr)
	}
	fmt.Fprint(w, "\t\t\t},\n")
	fmt.Fprint(w, "\t\t)\n")
	fmt.Fprintf(w, "\t\tif callErr_ != nil { %s }\n", returnWithErrExpr(m.Result, "fmt.Errorf(\"wacogo/witgen: CallRaw failed: %w\", callErr_)"))
	if needsResErr {
		fmt.Fprintf(w, "\t\tif resErr_ != nil { %s }\n", returnWithErrExpr(m.Result, "resErr_"))
	}
	fmt.Fprint(w, "\t}\n")
	if m.Result == nil {
		fmt.Fprint(w, "\treturn nil\n")
	} else {
		fmt.Fprint(w, "\treturn resVal_, nil\n")
	}
}

// emitToGoFlat writes the body of toGoFlat<Name>(ctx, cc, h, ty, stack []uint64) (T, error).
// Decodes the wire bytes already placed in stack by canon's transfer; does
// no canon-table handoff (resource handles arrive in cc's representation).
func emitToGoFlat(w *strings.Builder, t Type) {
	helperName := "toGoFlat" + TypeName(t)
	sink := errSinkLift{ZeroValueExpr: ZeroValueExpr(t)}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "_ = ty\n")
		fmt.Fprintf(w, "return %s, nil\n", liftFlatExpr(v, "stack[0]"))
	case TypeString:
		fmt.Fprintf(w, "_ = ty\n")
		fmt.Fprintf(w, `ptr_ := uint32(stack[0])
ln_ := uint32(stack[1])
bs_, ok_ := cc.Memory().Read(ptr_, ln_)
if !ok_ {
%s
}
return string(bs_), nil
`, sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory read")`, helperName)))
	case *TypeList:
		elemTy := GoTypeOf(v.Elem)
		elemSize := Size(v.Elem)
		fmt.Fprintf(w, "lstT_ := ty.(wacogo.TypeList)\n")
		fmt.Fprintf(w, "_ = lstT_\n")
		fmt.Fprintf(w, "ptr_ := uint32(stack[0])\n")
		fmt.Fprintf(w, "ln_ := uint32(stack[1])\n")
		fmt.Fprintf(w, "out_ := make([]%s, ln_)\n", elemTy)
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemReadIntoToGo(w, v.Elem, "out_[i_]", fmt.Sprintf("ptr_ + i_*%d", elemSize), helperName, "elem", sink, "\t", "lstT_.Elem")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "return out_, nil\n")
	case *TypeTuple:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "tupT_ := ty.(wacogo.TypeTuple)\n")
		fmt.Fprintf(w, "_ = tupT_\n")
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f)
			dst := fmt.Sprintf("v_.F%d", i)
			emitFlatSlotReadIntoToGo(w, f, dst, offset, n, helperName, fmt.Sprintf("field F%d", i), sink, "", fmt.Sprintf("tupT_.Types[%d]", i))
			offset += n
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeRecord:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "recT_ := ty.(wacogo.TypeRecord)\n")
		fmt.Fprintf(w, "_ = recT_\n")
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f.Type)
			dst := fmt.Sprintf("v_.%s", f.GoName)
			emitFlatSlotReadIntoToGo(w, f.Type, dst, offset, n, helperName, "field "+f.GoName, sink, "", fmt.Sprintf("recT_.Fields[%d].Type", i))
			offset += n
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeOption:
		emitToGoFlatOption(w, v, helperName, sink)
	case *TypeResult:
		emitToGoFlatResult(w, v, helperName, sink)
	case *TypeVariant:
		emitToGoFlatVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitToGoFlat: unhandled %T", t))
	}
}

// emitToGoMem writes the body of toGoMem<Name>(ctx, cc, h, ty, ptr uint32) (T, error).
// Same conventions as emitToGoFlat but reads from caller memory at ptr.
func emitToGoMem(w *strings.Builder, t Type) {
	helperName := "toGoMem" + TypeName(t)
	sink := errSinkLift{ZeroValueExpr: ZeroValueExpr(t)}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "_ = ty\n")
		fmt.Fprintf(w, "var out_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "%s\n", primMemReadStmt(v, "out_", "ptr", "cc.Memory()", helperName, "primitive", sink))
		fmt.Fprintf(w, "return out_, nil\n")
	case TypeString:
		fmt.Fprintf(w, "_ = ty\n")
		fmt.Fprintf(w, "var sptr_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "sptr_", "ptr", "cc.Memory()", helperName, "string ptr", sink))
		fmt.Fprintf(w, "var ln_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "ln_", "ptr + 4", "cc.Memory()", helperName, "string len", sink))
		fmt.Fprintf(w, `bs_, ok_ := cc.Memory().Read(sptr_, ln_)
if !ok_ {
%s
}
return string(bs_), nil
`, sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory read")`, helperName)))
	case *TypeList:
		elemTy := GoTypeOf(v.Elem)
		elemSize := Size(v.Elem)
		fmt.Fprintf(w, "lstT_ := ty.(wacogo.TypeList)\n")
		fmt.Fprintf(w, "_ = lstT_\n")
		fmt.Fprintf(w, "var lptr_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "lptr_", "ptr", "cc.Memory()", helperName, "list ptr", sink))
		fmt.Fprintf(w, "var ln_ uint32\n")
		fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU32, "ln_", "ptr + 4", "cc.Memory()", helperName, "list len", sink))
		fmt.Fprintf(w, "out_ := make([]%s, ln_)\n", elemTy)
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemReadIntoToGo(w, v.Elem, "out_[i_]", fmt.Sprintf("lptr_ + i_*%d", elemSize), helperName, "list elem", sink, "\t", "lstT_.Elem")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "return out_, nil\n")
	case *TypeTuple:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "tupT_ := ty.(wacogo.TypeTuple)\n")
		fmt.Fprintf(w, "_ = tupT_\n")
		for i, f := range v.Fields {
			off := FieldOffset(v.Fields, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemReadIntoToGo(w, f, fmt.Sprintf("v_.F%d", i), ptrExpr, helperName, fmt.Sprintf("field F%d", i), sink, "", fmt.Sprintf("tupT_.Types[%d]", i))
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeRecord:
		fmt.Fprintf(w, "var v_ %s\n", GoTypeOf(t))
		fmt.Fprintf(w, "recT_ := ty.(wacogo.TypeRecord)\n")
		fmt.Fprintf(w, "_ = recT_\n")
		fieldTys := recordFieldTypes(v)
		for i, f := range v.Fields {
			off := FieldOffset(fieldTys, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemReadIntoToGo(w, f.Type, fmt.Sprintf("v_.%s", f.GoName), ptrExpr, helperName, "field "+f.GoName, sink, "", fmt.Sprintf("recT_.Fields[%d].Type", i))
		}
		fmt.Fprint(w, "return v_, nil\n")
	case *TypeOption:
		emitToGoMemOption(w, v, helperName, sink)
	case *TypeResult:
		emitToGoMemResult(w, v, helperName, sink)
	case *TypeVariant:
		emitToGoMemVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitToGoMem: unhandled %T", t))
	}
}

// emitFromGoFlat writes the body of fromGoFlat<Name>(ctx, cc, h, stack []uint64, v T) error.
// Encodes the Go value v into wire bytes on stack; resource handles are
// bound to cc and invalidated on success.
func emitFromGoFlat(w *strings.Builder, t Type) {
	helperName := "fromGoFlat" + TypeName(t)
	sink := errSinkLower{}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "stack[0] = %s\n", lowerFlatExpr(v, "v"))
	case TypeString:
		fmt.Fprintf(w, `bs_ := []byte(v)
ptr_, err_ := cc.Realloc(ctx, 0, 0, 1, uint32(len(bs_)))
if err_ != nil {
%s
}
if ok_ := cc.Memory().Write(ptr_, bs_); !ok_ {
%s
}
stack[0] = uint64(ptr_)
stack[1] = uint64(len(bs_))
`,
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)),
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory write")`, helperName)),
		)
	case *TypeList:
		elemSize := Size(v.Elem)
		elemAlign := Align(v.Elem)
		fmt.Fprintf(w, "ln_ := uint32(len(v))\n")
		fmt.Fprintf(w, "ptr_, err_ := cc.Realloc(ctx, 0, 0, %d, ln_*%d)\n", elemAlign, elemSize)
		fmt.Fprintf(w, "if err_ != nil {\n%s\n}\n",
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)))
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemWriteFromGo(w, v.Elem, "v[i_]", fmt.Sprintf("ptr_ + i_*%d", elemSize), helperName, "list elem", sink, "\t")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "stack[0] = uint64(ptr_)\n")
		fmt.Fprintf(w, "stack[1] = uint64(ln_)\n")
	case *TypeTuple:
		offset := 0
		for i, f := range v.Fields {
			n := FlatSlots(f)
			emitFlatSlotWriteFromGo(w, f, fmt.Sprintf("v.F%d", i), offset, n, helperName, fmt.Sprintf("field F%d", i), sink, "")
			offset += n
		}
	case *TypeRecord:
		offset := 0
		for _, f := range v.Fields {
			n := FlatSlots(f.Type)
			emitFlatSlotWriteFromGo(w, f.Type, fmt.Sprintf("v.%s", f.GoName), offset, n, helperName, "field "+f.GoName, sink, "")
			offset += n
		}
	case *TypeOption:
		emitFromGoFlatOption(w, v, helperName, sink)
	case *TypeResult:
		emitFromGoFlatResult(w, v, helperName, sink)
	case *TypeVariant:
		emitFromGoFlatVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitFromGoFlat: unhandled %T", t))
	}
}

// emitFromGoMem writes the body of fromGoMem<Name>(ctx, cc, h, ptr uint32, v T) error.
// Same conventions as emitFromGoFlat but writes to caller memory at ptr.
func emitFromGoMem(w *strings.Builder, t Type) {
	helperName := "fromGoMem" + TypeName(t)
	sink := errSinkLower{}
	switch v := t.(type) {
	case Prim:
		fmt.Fprintf(w, "%s\n", primMemWriteStmtErr(v, "ptr", "v", "cc.Memory()", helperName, "primitive", sink))
	case TypeString:
		fmt.Fprintf(w, `bs_ := []byte(v)
sptr_, err_ := cc.Realloc(ctx, 0, 0, 1, uint32(len(bs_)))
if err_ != nil {
%s
}
if ok_ := cc.Memory().Write(sptr_, bs_); !ok_ {
%s
}
%s
%s
`,
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)),
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: bad memory write")`, helperName)),
			primMemWriteStmtErr(PrimU32, "ptr", "sptr_", "cc.Memory()", helperName, "string ptr", sink),
			primMemWriteStmtErr(PrimU32, "ptr + 4", "uint32(len(bs_))", "cc.Memory()", helperName, "string len", sink),
		)
	case *TypeList:
		elemSize := Size(v.Elem)
		elemAlign := Align(v.Elem)
		fmt.Fprintf(w, "ln_ := uint32(len(v))\n")
		fmt.Fprintf(w, "lptr_, err_ := cc.Realloc(ctx, 0, 0, %d, ln_*%d)\n", elemAlign, elemSize)
		fmt.Fprintf(w, "if err_ != nil {\n%s\n}\n",
			sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: realloc failed: %%w", err_)`, helperName)))
		fmt.Fprintf(w, "for i_ := uint32(0); i_ < ln_; i_++ {\n")
		emitElemWriteFromGo(w, v.Elem, "v[i_]", fmt.Sprintf("lptr_ + i_*%d", elemSize), helperName, "list elem", sink, "\t")
		fmt.Fprintf(w, "}\n")
		fmt.Fprintf(w, "%s\n", primMemWriteStmtErr(PrimU32, "ptr", "lptr_", "cc.Memory()", helperName, "list ptr", sink))
		fmt.Fprintf(w, "%s\n", primMemWriteStmtErr(PrimU32, "ptr + 4", "ln_", "cc.Memory()", helperName, "list len", sink))
	case *TypeTuple:
		for i, f := range v.Fields {
			off := FieldOffset(v.Fields, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemWriteFromGo(w, f, fmt.Sprintf("v.F%d", i), ptrExpr, helperName, fmt.Sprintf("field F%d", i), sink, "")
		}
	case *TypeRecord:
		fieldTys := recordFieldTypes(v)
		for i, f := range v.Fields {
			off := FieldOffset(fieldTys, i)
			ptrExpr := "ptr"
			if off > 0 {
				ptrExpr = fmt.Sprintf("ptr + %d", off)
			}
			emitElemWriteFromGo(w, f.Type, fmt.Sprintf("v.%s", f.GoName), ptrExpr, helperName, "field "+f.GoName, sink, "")
		}
	case *TypeOption:
		emitFromGoMemOption(w, v, helperName, sink)
	case *TypeResult:
		emitFromGoMemResult(w, v, helperName, sink)
	case *TypeVariant:
		emitFromGoMemVariant(w, v, helperName, sink)
	default:
		panic(fmt.Sprintf("witgen: emitFromGoMem: unhandled %T", t))
	}
}

// emitElemReadIntoToGo emits a memory read for one element of type elemTy
// into dstVar at memory address ptrExpr. Primitives, enums, flags and
// resources inline; compound types call the type's toGoMem helper.
// tyExpr is a Go expression evaluating at runtime to the wacogo.Type
// for elemTy (used to resolve resource-leaf TR identity through the
// trampoline's func signature).
func emitElemReadIntoToGo(w *strings.Builder, elemTy Type, dstVar, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	switch et := elemTy.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s\n", indent, primMemReadStmt(et, dstVar, ptrExpr, "cc.Memory()", helperName, op, sink))
	case *TypeEnum, *TypeFlags:
		fmt.Fprintf(w, "%s%s\n", indent, elemMemReadStmt(elemTy, dstVar, ptrExpr, "cc.Memory()", helperName, op, sink))
	case *TypeOwn:
		emitToGoMemOwnRead(w, et.Resource, dstVar, ptrExpr, helperName, op, sink, indent, tyExpr)
	case *TypeBorrow:
		emitToGoMemBorrowRead(w, et.Resource, dstVar, ptrExpr, helperName, op, sink, indent, tyExpr)
	default:
		// Compound element: dispatch to toGoMem<ElemName>.
		fmt.Fprintf(w, "%sif tmp_, err_ := toGoMem%s(ctx, cc, h, %s, %s); err_ != nil {\n%s\t%s\n%s} else {\n%s\t%s = tmp_\n%s}\n",
			indent, TypeName(elemTy), tyExpr, ptrExpr,
			indent, sink.Emit("err_"),
			indent,
			indent, dstVar,
			indent)
	}
}

// emitFlatSlotReadIntoToGo emits a flat-stack read for one element of type
// ty into dstVar from slots [slotOff, slotOff+slotN). Single-slot types
// inline; multi-slot compounds call the type's toGoFlat helper.
// tyExpr has the same role as in emitElemReadIntoToGo.
func emitFlatSlotReadIntoToGo(w *strings.Builder, ty Type, dstVar string, slotOff, slotN int, helperName, op string, sink errSink, indent, tyExpr string) {
	switch tt := ty.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s = %s\n", indent, dstVar, liftFlatExpr(tt, fmt.Sprintf("stack[%d]", slotOff)))
	case *TypeEnum:
		fmt.Fprintf(w, "%s%s = %s\n", indent, dstVar, liftFlatEnumExpr(tt, fmt.Sprintf("stack[%d]", slotOff)))
	case *TypeFlags:
		fmt.Fprintf(w, "%s%s = %s\n", indent, dstVar, liftFlatFlagsExpr(tt, fmt.Sprintf("stack[%d]", slotOff)))
	case *TypeOwn:
		emitToGoFlatOwnRead(w, tt.Resource, dstVar, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent, tyExpr)
	case *TypeBorrow:
		emitToGoFlatBorrowRead(w, tt.Resource, dstVar, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent, tyExpr)
	default:
		// Multi-slot compound: call toGoFlat<X>.
		fmt.Fprintf(w, "%sif tmp_, err_ := toGoFlat%s(ctx, cc, h, %s, stack[%d:%d]); err_ != nil {\n%s\t%s\n%s} else {\n%s\t%s = tmp_\n%s}\n",
			indent, TypeName(ty), tyExpr, slotOff, slotOff+slotN,
			indent, sink.Emit("err_"),
			indent,
			indent, dstVar,
			indent)
	}
}

// emitElemWriteFromGo emits a memory write for one element of type elemTy
// from valExpr at memory address ptrExpr. Primitives, enums, flags and
// resources inline; compound types call the type's fromGoMem helper.
func emitElemWriteFromGo(w *strings.Builder, elemTy Type, valExpr, ptrExpr, helperName, op string, sink errSink, indent string) {
	switch et := elemTy.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s\n", indent, primMemWriteStmtErr(et, ptrExpr, valExpr, "cc.Memory()", helperName, op, sink))
	case *TypeEnum, *TypeFlags:
		fmt.Fprintf(w, "%s%s\n", indent, elemMemWriteStmtErr(elemTy, ptrExpr, valExpr, "cc.Memory()", helperName, op, sink))
	case *TypeOwn:
		emitFromGoMemOwnWrite(w, et.Resource, valExpr, ptrExpr, helperName, op, sink, indent)
	case *TypeBorrow:
		emitFromGoMemBorrowWrite(w, et.Resource, valExpr, ptrExpr, helperName, op, sink, indent)
	default:
		// Compound element: dispatch to fromGoMem<ElemName>.
		fmt.Fprintf(w, "%sif err_ := fromGoMem%s(ctx, cc, h, %s, %s); err_ != nil {\n%s\t%s\n%s}\n",
			indent, TypeName(elemTy), ptrExpr, valExpr,
			indent, sink.Emit("err_"),
			indent)
	}
}

// emitFlatSlotWriteFromGo emits a flat-stack write for one element of type
// ty from valExpr into slots [slotOff, slotOff+slotN). Single-slot types
// inline; multi-slot compounds call the type's fromGoFlat helper.
func emitFlatSlotWriteFromGo(w *strings.Builder, ty Type, valExpr string, slotOff, slotN int, helperName, op string, sink errSink, indent string) {
	switch tt := ty.(type) {
	case Prim:
		fmt.Fprintf(w, "%sstack[%d] = %s\n", indent, slotOff, lowerFlatExpr(tt, valExpr))
	case *TypeEnum:
		fmt.Fprintf(w, "%sstack[%d] = %s\n", indent, slotOff, lowerFlatEnumExpr(tt, valExpr))
	case *TypeFlags:
		fmt.Fprintf(w, "%sstack[%d] = %s\n", indent, slotOff, lowerFlatFlagsExpr(tt, valExpr))
	case *TypeOwn:
		emitFromGoFlatOwnWrite(w, tt.Resource, valExpr, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent)
	case *TypeBorrow:
		emitFromGoFlatBorrowWrite(w, tt.Resource, valExpr, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent)
	default:
		// Multi-slot compound: call fromGoFlat<X>.
		fmt.Fprintf(w, "%sif err_ := fromGoFlat%s(ctx, cc, h, stack[%d:%d], %s); err_ != nil {\n%s\t%s\n%s}\n",
			indent, TypeName(ty), slotOff, slotOff+slotN, valExpr,
			indent, sink.Emit("err_"),
			indent)
	}
}

// emitNewHandleFromLocal emits the construction of a local *<R>Handle
// from an in-scope core.ResourceHandle expression. Resolves impl via
// the defining instance's extern table; nil impl is acceptable.
func emitNewHandleFromLocal(w *strings.Builder, rhExpr, dstVar string, rt *TypeResource, indent string) {
	fmt.Fprintf(w, "%svar impl_ %s\n", indent, rt.WrapName)
	fmt.Fprintf(w, "%sif obj_, ok_ := %s.Type().Instance().LookupExtern(%s.Rep()); ok_ {\n", indent, rhExpr, rhExpr)
	fmt.Fprintf(w, "%s\tif typed_, ok2_ := obj_.(%s); ok2_ { impl_ = typed_ }\n", indent, rt.WrapName)
	fmt.Fprintf(w, "%s}\n", indent)
	fmt.Fprintf(w, "%s%s = New%sHandleFrom(%s, impl_)\n", indent, dstVar, rt.WrapName, rhExpr)
}

// emitNewHandleFromForeign emits the construction of a foreign
// *<pkg>.<R>Handle by wrapping the rh in a forwarder backed by the
// importer's precomputed fns table on instanceState. Caller must
// already have state_ in scope.
func emitNewHandleFromForeign(w *strings.Builder, rhExpr, dstVar, callerExpr string, rt *TypeResource, indent string) {
	fmt.Fprintf(w, "%sfwd_ := &%s{caller: %s, rh: %s, fns: state_.%s}\n",
		indent,
		rt.GoPackageQualifier+rt.WrapName+"Forwarder",
		callerExpr, rhExpr,
		rt.GoPackageQualifier+rt.WrapName+"FwdFns",
	)
	fmt.Fprintf(w, "%s%s = %s.New%sHandleFrom(%s, fwd_)\n",
		indent, dstVar, rt.GoPackageQualifier, rt.WrapName, rhExpr)
}

// emitShortcutBorrowLift emits the lift body for a borrow<R> where R is
// locally defined. Reads oh_ from slotExpr, then defers to the rep-only
// resolution shared with the mem-read form.
func emitShortcutBorrowLift(w *strings.Builder, rt *TypeResource, dstVar, slotExpr, op string, sink errSink, indent string) {
	fmt.Fprintf(w, "%soh_ := uint32(%s)\n", indent, slotExpr)
	emitShortcutBorrowLiftFromOh(w, rt, dstVar, op, sink, indent)
}

// emitShortcutBorrowLiftFromOh emits the rep-resolution + handle
// construction for a borrow<R> received via the same-component shortcut.
// oh_ must already be declared in scope. Resolves the impl via h's
// extern table and constructs a *<R>Handle in the shortcut-borrow state
// so LocalImpl returns the impl directly without reaching the canon
// table.
func emitShortcutBorrowLiftFromOh(w *strings.Builder, rt *TypeResource, dstVar, op string, sink errSink, indent string) {
	fmt.Fprintf(w, "%sobj_, ok_ := h.LookupResource(host.ExternHandle(oh_))\n", indent)
	fmt.Fprintf(w, "%sif !ok_ {\n%s\t%s\n%s}\n", indent, indent,
		sink.Emit(fmt.Sprintf("fmt.Errorf(\"wacogo/witgen: %s: unknown rep %%d\", oh_)", op)), indent)
	fmt.Fprintf(w, "%simpl_, _ := obj_.(%s)\n", indent, rt.WrapName)
	fmt.Fprintf(w, "%s%s = &%sHandle{\n", indent, dstVar, rt.WrapName)
	fmt.Fprintf(w, "%s\t%s: impl_,\n", indent, rt.WrapName)
	fmt.Fprintf(w, "%s\tstate: &shortcutBorrowHandleState{defining: h, rep: oh_, impl: obj_},\n", indent)
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitToGoFlatOwnRead emits the decode for an own<R> handle from a flat
// stack slot. The wire form is the canon handle on cc's table; we look
// up the live core.ResourceHandle and wrap it via New<R>HandleFrom.
//
// tyExpr is a Go expression evaluating at runtime to the wacogo.TypeOwn
// for this leaf — its ResourceType identifies the per-instance TR
// corresponding to this position in the trampoline's signature.
func emitToGoFlatOwnRead(w *strings.Builder, rt *TypeResource, dstVar, slotExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	_, _ = helperName, op
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%soh_ := uint32(%s)\n", inner, slotExpr)
	fmt.Fprintf(w, "%stbl_ := %s.(wacogo.TypeOwn).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%srh_, lookupErr_ := cc.LookupBorrowable(tbl_, oh_)\n", inner)
	fmt.Fprintf(w, "%sif lookupErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("lookupErr_"), inner)
	if rt.IsImported() {
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "rh_", dstVar, "h", rt, inner)
	} else {
		emitNewHandleFromLocal(w, "rh_", dstVar, rt, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitToGoFlatBorrowRead emits the decode for a borrow<R> handle from a
// flat stack slot. For an imported R the wire form is a canon handle in
// cc's table — look up via LookupBorrowable. For a locally-defined R
// the canonical-ABI same-component shortcut applies (callee == defining):
// the wire value is the rep, with no callee-side table entry; resolve
// the impl through h's extern table and wrap in a shortcut-borrow state.
//
// See emitToGoFlatOwnRead for tyExpr semantics; here the runtime type is
// wacogo.TypeBorrow.
func emitToGoFlatBorrowRead(w *strings.Builder, rt *TypeResource, dstVar, slotExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	_ = helperName
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%soh_ := uint32(%s)\n", inner, slotExpr)
		fmt.Fprintf(w, "%stbl_ := %s.(wacogo.TypeBorrow).ResourceType\n", inner, tyExpr)
		fmt.Fprintf(w, "%srh_, lookupErr_ := cc.LookupBorrowable(tbl_, oh_)\n", inner)
		fmt.Fprintf(w, "%sif lookupErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("lookupErr_"), inner)
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "rh_", dstVar, "h", rt, inner)
	} else {
		emitShortcutBorrowLift(w, rt, dstVar, slotExpr, op, sink, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitToGoMemOwnRead is the memory-read form of emitToGoFlatOwnRead.
func emitToGoMemOwnRead(w *strings.Builder, rt *TypeResource, dstVar, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%svar oh_ uint32\n", inner)
	fmt.Fprintf(w, "%s%s\n", inner, primMemReadStmt(PrimU32, "oh_", ptrExpr, "cc.Memory()", helperName, op+": handle", sink))
	fmt.Fprintf(w, "%stbl_ := %s.(wacogo.TypeOwn).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%srh_, lookupErr_ := cc.LookupBorrowable(tbl_, oh_)\n", inner)
	fmt.Fprintf(w, "%sif lookupErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("lookupErr_"), inner)
	if rt.IsImported() {
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "rh_", dstVar, "h", rt, inner)
	} else {
		emitNewHandleFromLocal(w, "rh_", dstVar, rt, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitToGoMemBorrowRead is the memory-read form of emitToGoFlatBorrowRead.
func emitToGoMemBorrowRead(w *strings.Builder, rt *TypeResource, dstVar, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%svar oh_ uint32\n", inner)
	fmt.Fprintf(w, "%s%s\n", inner, primMemReadStmt(PrimU32, "oh_", ptrExpr, "cc.Memory()", helperName, op+": handle", sink))
	if rt.IsImported() {
		fmt.Fprintf(w, "%stbl_ := %s.(wacogo.TypeBorrow).ResourceType\n", inner, tyExpr)
		fmt.Fprintf(w, "%srh_, lookupErr_ := cc.LookupBorrowable(tbl_, oh_)\n", inner)
		fmt.Fprintf(w, "%sif lookupErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("lookupErr_"), inner)
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "rh_", dstVar, "h", rt, inner)
	} else {
		emitShortcutBorrowLiftFromOh(w, rt, dstVar, op, sink, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitFromGoFlatOwnWrite emits the encode for an own<R> handle into a flat
// stack slot. v.bind(cc, h) for local, <pkg>.HandleValueFor for foreign;
// stack receives the canon handle and v is invalidated.
func emitFromGoFlatOwnWrite(w *strings.Builder, rt *TypeResource, valExpr, slotExpr, helperName, op string, sink errSink, indent string) {
	_ = op
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%soh_, err_ := %s.HandleValueFor(cc, h, %s)\n", inner, rt.GoPackageQualifier, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
		fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, valExpr)
		fmt.Fprintf(w, "%s%s = uint64(oh_)\n", inner, slotExpr)
	} else {
		fmt.Fprintf(w, "%soh_, err_ := %s.bind(cc, h)\n", inner, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
		fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, valExpr)
		fmt.Fprintf(w, "%s%s = uint64(oh_)\n", inner, slotExpr)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitFromGoFlatBorrowWrite emits an error: borrow<R> may not appear in
// result position in well-formed WIT.
func emitFromGoFlatBorrowWrite(w *strings.Builder, rt *TypeResource, valExpr, slotExpr, helperName, op string, sink errSink, indent string) {
	_ = op
	fmt.Fprintf(w, "%s_ = %s\n", indent, valExpr)
	fmt.Fprintf(w, "%s_ = %s\n", indent, slotExpr)
	fmt.Fprintf(w, "%s%s\n", indent,
		sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: borrow<%s> in result position is invalid")`, helperName, rt.WrapName)))
}

// emitFromGoMemOwnWrite is the memory-write form of emitFromGoFlatOwnWrite.
// Writes the resulting canon handle as a u32 to memory at ptrExpr.
func emitFromGoMemOwnWrite(w *strings.Builder, rt *TypeResource, valExpr, ptrExpr, helperName, op string, sink errSink, indent string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%soh_, err_ := %s.HandleValueFor(cc, h, %s)\n", inner, rt.GoPackageQualifier, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
		fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, valExpr)
		fmt.Fprintf(w, "%s%s\n", inner, primMemWriteStmtErr(PrimU32, ptrExpr, "oh_", "cc.Memory()", helperName, op+": handle", sink))
	} else {
		fmt.Fprintf(w, "%soh_, err_ := %s.bind(cc, h)\n", inner, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
		fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, valExpr)
		fmt.Fprintf(w, "%s%s\n", inner, primMemWriteStmtErr(PrimU32, ptrExpr, "oh_", "cc.Memory()", helperName, op+": handle", sink))
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitFromGoMemBorrowWrite emits an error: borrow<R> may not appear in
// result position in well-formed WIT.
func emitFromGoMemBorrowWrite(w *strings.Builder, rt *TypeResource, valExpr, ptrExpr, helperName, op string, sink errSink, indent string) {
	_ = op
	fmt.Fprintf(w, "%s_ = %s\n", indent, valExpr)
	fmt.Fprintf(w, "%s_ = %s\n", indent, ptrExpr)
	fmt.Fprintf(w, "%s%s\n", indent,
		sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: borrow<%s> in result position is invalid")`, helperName, rt.WrapName)))
}

// emitToGoFlatOption emits the body for a *TypeOption in flat mode.
// Option uses an OldX{IsSome, Value} struct shape (Result uses the
// sealed-interface shape; Option keeps its own struct shape).
func emitToGoFlatOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLift) {
	elemSlots := FlatSlots(v.Elem)
	fmt.Fprintf(w, "optT_ := ty.(wacogo.TypeOption)\n")
	fmt.Fprintf(w, "_ = optT_\n")
	fmt.Fprintf(w, "disc_ := uint32(stack[0])\n")
	fmt.Fprintf(w, "var out_ %s\n", GoTypeOf(v))
	fmt.Fprintf(w, "if disc_ != 0 {\n")
	fmt.Fprintf(w, "\tout_.IsSome = true\n")
	emitFlatSlotReadIntoToGo(w, v.Elem, "out_.Value", 1, elemSlots, helperName, "Some payload", sink, "\t", "optT_.Inner")
	fmt.Fprintf(w, "}\n")
	fmt.Fprint(w, "return out_, nil\n")
}

// emitToGoFlatResult emits the body for a *TypeResult in flat mode using
// the sealed iface shape (<Name>Ok, <Name>Err case structs).
func emitToGoFlatResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLift) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	fmt.Fprintf(w, "resT_ := ty.(wacogo.TypeResult)\n")
	fmt.Fprintf(w, "_ = resT_\n")
	fmt.Fprintf(w, "disc_ := uint32(stack[0])\n")
	fmt.Fprintf(w, "if disc_ == 0 {\n")
	if v.OK != nil {
		okSlots := FlatSlots(v.OK)
		fmt.Fprintf(w, "\tvar okVal_ %s\n", GoTypeOf(v.OK))
		emitFlatSlotReadIntoToGo(w, v.OK, "okVal_", 1, okSlots, helperName, "OK arm", sink, "\t", "resT_.Ok")
		fmt.Fprintf(w, "\treturn %s{Value: okVal_}, nil\n", okName)
	} else {
		fmt.Fprintf(w, "\treturn %s{}, nil\n", okName)
	}
	fmt.Fprintf(w, "}\n")
	if v.Err != nil {
		errSlots := FlatSlots(v.Err)
		fmt.Fprintf(w, "var errVal_ %s\n", GoTypeOf(v.Err))
		emitFlatSlotReadIntoToGo(w, v.Err, "errVal_", 1, errSlots, helperName, "Err arm", sink, "", "resT_.Err")
		fmt.Fprintf(w, "return %s{Value: errVal_}, nil\n", errName)
	} else {
		fmt.Fprintf(w, "return %s{}, nil\n", errName)
	}
}

// emitToGoFlatVariant emits the body for a *TypeVariant in flat mode.
func emitToGoFlatVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLift) {
	fmt.Fprint(w, "varT_ := ty.(wacogo.TypeVariant)\n")
	fmt.Fprint(w, "_ = varT_\n")
	fmt.Fprint(w, "disc_ := uint32(stack[0])\n")
	fmt.Fprint(w, "switch disc_ {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %d:\n", i)
		if c.Payload == nil {
			fmt.Fprintf(w, "\treturn %s{}, nil\n", variantCaseName(v, c))
			continue
		}
		payloadSlots := FlatSlots(c.Payload)
		fmt.Fprintf(w, "\tvar val %s\n", GoTypeOf(c.Payload))
		emitFlatSlotReadIntoToGo(w, c.Payload, "val", 1, payloadSlots, helperName, fmt.Sprintf("case %d payload", i), sink, "\t", fmt.Sprintf("varT_.Cases[%d].Payload", i))
		fmt.Fprintf(w, "\treturn %s{Value: val}, nil\n", variantCaseName(v, c))
	}
	fmt.Fprint(w, "}\n")
	fmt.Fprintf(w, "return %s, fmt.Errorf(\"wacogo/witgen: %s: invalid discriminant %%d\", disc_)\n", ZeroValueExpr(v), helperName)
}

// emitToGoMemOption is the memory-mode counterpart of emitToGoFlatOption.
func emitToGoMemOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLift) {
	payloadOffset := alignUp(1, Align(v.Elem))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "optT_ := ty.(wacogo.TypeOption)\n")
	fmt.Fprintf(w, "_ = optT_\n")
	fmt.Fprintf(w, "var disc_ uint8\n")
	fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU8, "disc_", "ptr", "cc.Memory()", helperName, "discriminant", sink))
	fmt.Fprintf(w, "var out_ %s\n", GoTypeOf(v))
	fmt.Fprintf(w, "if disc_ != 0 {\n")
	fmt.Fprintf(w, "\tout_.IsSome = true\n")
	emitElemReadIntoToGo(w, v.Elem, "out_.Value", payloadPtr, helperName, "Some payload", sink, "\t", "optT_.Inner")
	fmt.Fprintf(w, "}\n")
	fmt.Fprint(w, "return out_, nil\n")
}

// emitToGoMemResult is the memory-mode counterpart of emitToGoFlatResult.
func emitToGoMemResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLift) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	payloadOffset := alignUp(1, resultPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "resT_ := ty.(wacogo.TypeResult)\n")
	fmt.Fprintf(w, "_ = resT_\n")
	fmt.Fprintf(w, "var disc_ uint8\n")
	fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU8, "disc_", "ptr", "cc.Memory()", helperName, "discriminant", sink))
	fmt.Fprintf(w, "if disc_ == 0 {\n")
	if v.OK != nil {
		fmt.Fprintf(w, "\tvar okVal_ %s\n", GoTypeOf(v.OK))
		emitElemReadIntoToGo(w, v.OK, "okVal_", payloadPtr, helperName, "OK arm", sink, "\t", "resT_.Ok")
		fmt.Fprintf(w, "\treturn %s{Value: okVal_}, nil\n", okName)
	} else {
		fmt.Fprintf(w, "\treturn %s{}, nil\n", okName)
	}
	fmt.Fprintf(w, "}\n")
	if v.Err != nil {
		fmt.Fprintf(w, "var errVal_ %s\n", GoTypeOf(v.Err))
		emitElemReadIntoToGo(w, v.Err, "errVal_", payloadPtr, helperName, "Err arm", sink, "", "resT_.Err")
		fmt.Fprintf(w, "return %s{Value: errVal_}, nil\n", errName)
	} else {
		fmt.Fprintf(w, "return %s{}, nil\n", errName)
	}
}

// emitToGoMemVariant is the memory-mode counterpart of emitToGoFlatVariant.
func emitToGoMemVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLift) {
	payloadOffset := alignUp(1, variantPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "varT_ := ty.(wacogo.TypeVariant)\n")
	fmt.Fprintf(w, "_ = varT_\n")
	fmt.Fprintf(w, "var disc_ uint8\n")
	fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU8, "disc_", "ptr", "cc.Memory()", helperName, "discriminant", sink))
	fmt.Fprint(w, "switch disc_ {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %d:\n", i)
		if c.Payload == nil {
			fmt.Fprintf(w, "\treturn %s{}, nil\n", variantCaseName(v, c))
			continue
		}
		fmt.Fprintf(w, "\tvar val %s\n", GoTypeOf(c.Payload))
		emitElemReadIntoToGo(w, c.Payload, "val", payloadPtr, helperName, fmt.Sprintf("case %d payload", i), sink, "\t", fmt.Sprintf("varT_.Cases[%d].Payload", i))
		fmt.Fprintf(w, "\treturn %s{Value: val}, nil\n", variantCaseName(v, c))
	}
	fmt.Fprint(w, "}\n")
	fmt.Fprintf(w, "return %s, fmt.Errorf(\"wacogo/witgen: %s: invalid discriminant %%d\", disc_)\n", ZeroValueExpr(v), helperName)
}

// emitFromGoFlatOption emits the body for a *TypeOption in flat encode mode.
func emitFromGoFlatOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLower) {
	elemSlots := FlatSlots(v.Elem)
	fmt.Fprintf(w, "if v.IsSome {\n")
	fmt.Fprintf(w, "\tstack[0] = 1\n")
	emitFlatSlotWriteFromGo(w, v.Elem, "v.Value", 1, elemSlots, helperName, "Some payload", sink, "\t")
	fmt.Fprintf(w, "} else {\n")
	fmt.Fprintf(w, "\tstack[0] = 0\n")
	for i := 0; i < elemSlots; i++ {
		fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
	}
	fmt.Fprintf(w, "}\n")
}

// emitFromGoFlatResult emits the body for a *TypeResult in flat encode
// mode using the sealed iface shape.
func emitFromGoFlatResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLower) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	okSlots := 0
	if v.OK != nil {
		okSlots = FlatSlots(v.OK)
	}
	errSlots := 0
	if v.Err != nil {
		errSlots = FlatSlots(v.Err)
	}
	joinedSlots := okSlots
	if errSlots > joinedSlots {
		joinedSlots = errSlots
	}
	fmt.Fprintf(w, "switch x := v.(type) {\n")
	fmt.Fprintf(w, "case %s:\n", okName)
	fmt.Fprintf(w, "\tstack[0] = 0\n")
	if v.OK != nil {
		emitFlatSlotWriteFromGo(w, v.OK, "x.Value", 1, okSlots, helperName, "OK arm", sink, "\t")
		for i := okSlots; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	} else {
		fmt.Fprint(w, "\t_ = x\n")
		for i := 0; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	}
	fmt.Fprintf(w, "case %s:\n", errName)
	fmt.Fprintf(w, "\tstack[0] = 1\n")
	if v.Err != nil {
		emitFlatSlotWriteFromGo(w, v.Err, "x.Value", 1, errSlots, helperName, "Err arm", sink, "\t")
		for i := errSlots; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	} else {
		fmt.Fprint(w, "\t_ = x\n")
		for i := 0; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	}
	fmt.Fprintf(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown result case")`, helperName)))
	fmt.Fprintf(w, "}\n")
}

// emitFromGoFlatVariant emits the body for a *TypeVariant in flat encode mode.
func emitFromGoFlatVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLower) {
	totalSlots := FlatSlots(v) - 1
	fmt.Fprint(w, "switch x := v.(type) {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %s:\n", variantCaseName(v, c))
		fmt.Fprintf(w, "\tstack[0] = %d\n", i)
		payloadSlots := 0
		if c.Payload != nil {
			payloadSlots = FlatSlots(c.Payload)
			emitFlatSlotWriteFromGo(w, c.Payload, "x.Value", 1, payloadSlots, helperName, fmt.Sprintf("case %d payload", i), sink, "\t")
		} else {
			fmt.Fprint(w, "\t_ = x\n")
		}
		for j := payloadSlots; j < totalSlots; j++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+j)
		}
	}
	fmt.Fprint(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown variant case")`, helperName)))
	fmt.Fprint(w, "}\n")
}

// emitFromGoMemOption is the memory-mode counterpart of emitFromGoFlatOption.
func emitFromGoMemOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLower) {
	payloadOffset := alignUp(1, Align(v.Elem))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "if v.IsSome {\n")
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "1", "cc.Memory()", helperName, "discriminant", sink))
	emitElemWriteFromGo(w, v.Elem, "v.Value", payloadPtr, helperName, "Some payload", sink, "\t")
	fmt.Fprintf(w, "} else {\n")
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "0", "cc.Memory()", helperName, "discriminant", sink))
	fmt.Fprintf(w, "}\n")
}

// emitFromGoMemResult is the memory-mode counterpart of emitFromGoFlatResult.
func emitFromGoMemResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLower) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	payloadOffset := alignUp(1, resultPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "switch x := v.(type) {\n")
	fmt.Fprintf(w, "case %s:\n", okName)
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "0", "cc.Memory()", helperName, "discriminant", sink))
	if v.OK != nil {
		emitElemWriteFromGo(w, v.OK, "x.Value", payloadPtr, helperName, "OK arm", sink, "\t")
	} else {
		fmt.Fprint(w, "\t_ = x\n")
	}
	fmt.Fprintf(w, "case %s:\n", errName)
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "1", "cc.Memory()", helperName, "discriminant", sink))
	if v.Err != nil {
		emitElemWriteFromGo(w, v.Err, "x.Value", payloadPtr, helperName, "Err arm", sink, "\t")
	} else {
		fmt.Fprint(w, "\t_ = x\n")
	}
	fmt.Fprintf(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown result case")`, helperName)))
	fmt.Fprintf(w, "}\n")
}

// emitFromGoMemVariant is the memory-mode counterpart of emitFromGoFlatVariant.
func emitFromGoMemVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLower) {
	payloadOffset := alignUp(1, variantPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprint(w, "switch x := v.(type) {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %s:\n", variantCaseName(v, c))
		fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", fmt.Sprintf("uint8(%d)", i), "cc.Memory()", helperName, "discriminant", sink))
		if c.Payload != nil {
			emitElemWriteFromGo(w, c.Payload, "x.Value", payloadPtr, helperName, fmt.Sprintf("case %d payload", i), sink, "\t")
		} else {
			fmt.Fprint(w, "\t_ = x\n")
		}
	}
	fmt.Fprint(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown variant case")`, helperName)))
	fmt.Fprint(w, "}\n")
}

// emitElemReadIntoLift emits a memory read for one element of type elemTy
// into dstVar at memory address ptrExpr (callee memory). Primitives,
// enums, flags and resources inline; compound types call the type's
// liftMem helper.
//
// tyExpr is a Go expression evaluating at runtime to the wacogo.Type
// for elemTy at this position in the wrapped function's signature; it
// flows into resource-leaf TR extraction and into nested helper calls.
func emitElemReadIntoLift(w *strings.Builder, elemTy Type, dstVar, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	switch et := elemTy.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s\n", indent, primMemReadStmt(et, dstVar, ptrExpr, "callee.Memory()", helperName, op, sink))
	case *TypeEnum, *TypeFlags:
		fmt.Fprintf(w, "%s%s\n", indent, elemMemReadStmt(elemTy, dstVar, ptrExpr, "callee.Memory()", helperName, op, sink))
	case *TypeOwn:
		emitLiftMemOwnRead(w, et.Resource, dstVar, ptrExpr, helperName, op, sink, indent, tyExpr)
	case *TypeBorrow:
		emitLiftMemBorrowRead(w, et.Resource, dstVar, ptrExpr, helperName, op, sink, indent, tyExpr)
	default:
		// Compound element: dispatch to liftMem<ElemName>.
		fmt.Fprintf(w, "%sif tmp_, err_ := liftMem%s(ctx, caller, callee, h, %s, %s); err_ != nil {\n%s\t%s\n%s} else {\n%s\t%s = tmp_\n%s}\n",
			indent, TypeName(elemTy), tyExpr, ptrExpr,
			indent, sink.Emit("err_"),
			indent,
			indent, dstVar,
			indent)
	}
}

// emitFlatSlotReadIntoLift emits a flat-stack read for one element of type
// ty into dstVar from slots [slotOff, slotOff+slotN). Single-slot types
// inline; multi-slot compounds call the type's liftFlat helper. tyExpr
// has the same role as in emitElemReadIntoLift.
func emitFlatSlotReadIntoLift(w *strings.Builder, ty Type, dstVar string, slotOff, slotN int, helperName, op string, sink errSink, indent, tyExpr string) {
	switch tt := ty.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s = %s\n", indent, dstVar, liftFlatExpr(tt, fmt.Sprintf("stack[%d]", slotOff)))
	case *TypeEnum:
		fmt.Fprintf(w, "%s%s = %s\n", indent, dstVar, liftFlatEnumExpr(tt, fmt.Sprintf("stack[%d]", slotOff)))
	case *TypeFlags:
		fmt.Fprintf(w, "%s%s = %s\n", indent, dstVar, liftFlatFlagsExpr(tt, fmt.Sprintf("stack[%d]", slotOff)))
	case *TypeOwn:
		emitLiftFlatOwnRead(w, tt.Resource, dstVar, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent, tyExpr)
	case *TypeBorrow:
		emitLiftFlatBorrowRead(w, tt.Resource, dstVar, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent, tyExpr)
	default:
		// Multi-slot compound: call liftFlat<X>.
		fmt.Fprintf(w, "%sif tmp_, err_ := liftFlat%s(ctx, caller, callee, h, %s, stack[%d:%d]); err_ != nil {\n%s\t%s\n%s} else {\n%s\t%s = tmp_\n%s}\n",
			indent, TypeName(ty), tyExpr, slotOff, slotOff+slotN,
			indent, sink.Emit("err_"),
			indent,
			indent, dstVar,
			indent)
	}
}

// emitElemWriteFromLower emits a memory write for one element of type
// elemTy from valExpr at memory address ptrExpr (callee memory).
// Primitives, enums, flags and resources inline; compound types call
// the type's lowerMem helper. tyExpr has the same role as in
// emitElemReadIntoLift.
func emitElemWriteFromLower(w *strings.Builder, elemTy Type, valExpr, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	switch et := elemTy.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s\n", indent, primMemWriteStmtErr(et, ptrExpr, valExpr, "callee.Memory()", helperName, op, sink))
	case *TypeEnum, *TypeFlags:
		fmt.Fprintf(w, "%s%s\n", indent, elemMemWriteStmtErr(elemTy, ptrExpr, valExpr, "callee.Memory()", helperName, op, sink))
	case *TypeOwn:
		emitLowerMemOwnWrite(w, et.Resource, valExpr, ptrExpr, helperName, op, sink, indent, tyExpr)
	case *TypeBorrow:
		emitLowerMemBorrowWrite(w, et.Resource, valExpr, ptrExpr, helperName, op, sink, indent, tyExpr)
	default:
		// Compound element: dispatch to lowerMem<ElemName>.
		fmt.Fprintf(w, "%sif err_ := lowerMem%s(ctx, caller, callee, h, %s, %s, %s); err_ != nil {\n%s\t%s\n%s}\n",
			indent, TypeName(elemTy), tyExpr, ptrExpr, valExpr,
			indent, sink.Emit("err_"),
			indent)
	}
}

// emitFlatSlotWriteFromLower emits a flat-stack write for one element of
// type ty from valExpr into slots [slotOff, slotOff+slotN). Single-slot
// types inline; multi-slot compounds call the type's lowerFlat helper.
// tyExpr has the same role as in emitElemReadIntoLift.
func emitFlatSlotWriteFromLower(w *strings.Builder, ty Type, valExpr string, slotOff, slotN int, helperName, op string, sink errSink, indent, tyExpr string) {
	switch tt := ty.(type) {
	case Prim:
		fmt.Fprintf(w, "%sstack[%d] = %s\n", indent, slotOff, lowerFlatExpr(tt, valExpr))
	case *TypeEnum:
		fmt.Fprintf(w, "%sstack[%d] = %s\n", indent, slotOff, lowerFlatEnumExpr(tt, valExpr))
	case *TypeFlags:
		fmt.Fprintf(w, "%sstack[%d] = %s\n", indent, slotOff, lowerFlatFlagsExpr(tt, valExpr))
	case *TypeOwn:
		emitLowerFlatOwnWrite(w, tt.Resource, valExpr, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent, tyExpr)
	case *TypeBorrow:
		emitLowerFlatBorrowWrite(w, tt.Resource, valExpr, fmt.Sprintf("stack[%d]", slotOff), helperName, op, sink, indent, tyExpr)
	default:
		// Multi-slot compound: call lowerFlat<X>.
		fmt.Fprintf(w, "%sif err_ := lowerFlat%s(ctx, caller, callee, h, %s, stack[%d:%d], %s); err_ != nil {\n%s\t%s\n%s}\n",
			indent, TypeName(ty), tyExpr, slotOff, slotOff+slotN, valExpr,
			indent, sink.Emit("err_"),
			indent)
	}
}

// emitLiftFlatOwnRead emits the decode for an own<R> handle from a flat
// stack slot on the wrap path. The handle is transferred from callee to
// caller via TransferOwn, then wrapped via New<R>HandleFrom.
//
// tyExpr is a Go expression evaluating at runtime to the wacogo.TypeOwn
// for this leaf — its ResourceType identifies the per-instance TR
// corresponding to this position in the wrapped function's signature.
func emitLiftFlatOwnRead(w *strings.Builder, rt *TypeResource, dstVar, slotExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	_, _ = helperName, op
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%scalleeOh_ := uint32(%s)\n", inner, slotExpr)
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeOwn).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%ssrcH_, srcErr_ := callee.LookupOwn(tr_, calleeOh_)\n", inner)
	fmt.Fprintf(w, "%sif srcErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("srcErr_"), inner)
	fmt.Fprintf(w, "%sdstH_, dstErr_ := srcH_.TransferOwn(caller.TransferTarget())\n", inner)
	fmt.Fprintf(w, "%sif dstErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("dstErr_"), inner)
	if rt.IsImported() {
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "dstH_", dstVar, "h", rt, inner)
	} else {
		emitNewHandleFromLocal(w, "dstH_", dstVar, rt, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLiftFlatBorrowRead emits the decode for a borrow<R> handle from a
// flat stack slot on the wrap path. The borrow is transferred from
// callee to caller via LendTo, then wrapped via New<R>HandleFrom.
//
// See emitLiftFlatOwnRead for tyExpr semantics; here the runtime type
// is wacogo.TypeBorrow.
func emitLiftFlatBorrowRead(w *strings.Builder, rt *TypeResource, dstVar, slotExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	_, _ = helperName, op
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%scalleeOh_ := uint32(%s)\n", inner, slotExpr)
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeBorrow).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%scalleeSrc_, srcErr_ := callee.LookupBorrowable(tr_, calleeOh_)\n", inner)
	fmt.Fprintf(w, "%sif srcErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("srcErr_"), inner)
	fmt.Fprintf(w, "%sdstH_, dstErr_ := calleeSrc_.LendTo(caller.TransferTarget(), caller.Task())\n", inner)
	fmt.Fprintf(w, "%sif dstErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("dstErr_"), inner)
	if rt.IsImported() {
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "dstH_", dstVar, "h", rt, inner)
	} else {
		emitNewHandleFromLocal(w, "dstH_", dstVar, rt, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLiftMemOwnRead is the memory-read form of emitLiftFlatOwnRead.
func emitLiftMemOwnRead(w *strings.Builder, rt *TypeResource, dstVar, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%svar calleeOh_ uint32\n", inner)
	fmt.Fprintf(w, "%s%s\n", inner, primMemReadStmt(PrimU32, "calleeOh_", ptrExpr, "callee.Memory()", helperName, op+": handle", sink))
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeOwn).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%ssrcH_, srcErr_ := callee.LookupOwn(tr_, calleeOh_)\n", inner)
	fmt.Fprintf(w, "%sif srcErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("srcErr_"), inner)
	fmt.Fprintf(w, "%sdstH_, dstErr_ := srcH_.TransferOwn(caller.TransferTarget())\n", inner)
	fmt.Fprintf(w, "%sif dstErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("dstErr_"), inner)
	if rt.IsImported() {
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "dstH_", dstVar, "h", rt, inner)
	} else {
		emitNewHandleFromLocal(w, "dstH_", dstVar, rt, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLiftMemBorrowRead is the memory-read form of emitLiftFlatBorrowRead.
func emitLiftMemBorrowRead(w *strings.Builder, rt *TypeResource, dstVar, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%svar calleeOh_ uint32\n", inner)
	fmt.Fprintf(w, "%s%s\n", inner, primMemReadStmt(PrimU32, "calleeOh_", ptrExpr, "callee.Memory()", helperName, op+": handle", sink))
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeBorrow).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%scalleeSrc_, srcErr_ := callee.LookupBorrowable(tr_, calleeOh_)\n", inner)
	fmt.Fprintf(w, "%sif srcErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("srcErr_"), inner)
	fmt.Fprintf(w, "%sdstH_, dstErr_ := calleeSrc_.LendTo(caller.TransferTarget(), caller.Task())\n", inner)
	fmt.Fprintf(w, "%sif dstErr_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("dstErr_"), inner)
	if rt.IsImported() {
		fmt.Fprintf(w, "%sstate_ := h.UserState().(*instanceState)\n", inner)
		emitNewHandleFromForeign(w, "dstH_", dstVar, "h", rt, inner)
	} else {
		emitNewHandleFromLocal(w, "dstH_", dstVar, rt, inner)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLowerFlatOwnWrite emits the encode for an own<R> handle into a flat
// stack slot on the wrap path. The handle is bound on the caller, then
// the rep is moved cross-table to the callee via LookupOwn → TransferOwn.
//
// tyExpr is a Go expression evaluating at runtime to the wacogo.TypeOwn
// for this leaf — its ResourceType identifies the per-instance TR for
// this position in the wrapped function's signature.
func emitLowerFlatOwnWrite(w *strings.Builder, rt *TypeResource, valExpr, slotExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	_ = op
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.HandleValueFor(caller, h, %s)\n", inner, rt.GoPackageQualifier, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	} else {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.bind(caller, h)\n", inner, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	}
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeOwn).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%ssrcH_, err2_ := caller.LookupOwn(tr_,callerH_)\n", inner)
	fmt.Fprintf(w, "%sif err2_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err2_"), inner)
	fmt.Fprintf(w, "%sdstH_, err3_ := srcH_.TransferOwn(callee.TransferTarget())\n", inner)
	fmt.Fprintf(w, "%sif err3_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err3_"), inner)
	fmt.Fprintf(w, "%s%s = uint64(dstH_.HandleID())\n", inner, slotExpr)
	fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, valExpr)
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLowerFlatBorrowWrite emits the encode for a borrow<R> handle into a
// flat stack slot on the wrap path. Caller binds; the borrow is
// transferred to the callee via LookupBorrowable+LendTo. The unlend
// release fires at call resolution via the task.
//
// See emitLowerFlatOwnWrite for tyExpr semantics; here the runtime type
// is wacogo.TypeBorrow.
func emitLowerFlatBorrowWrite(w *strings.Builder, rt *TypeResource, valExpr, slotExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	_ = op
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.HandleValueFor(caller, h, %s)\n", inner, rt.GoPackageQualifier, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	} else {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.bind(caller, h)\n", inner, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	}
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeBorrow).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%ssrcH_, err2_ := caller.LookupBorrowable(tr_,callerH_)\n", inner)
	fmt.Fprintf(w, "%sif err2_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err2_"), inner)
	fmt.Fprintf(w, "%sdstH_, err3_ := srcH_.LendTo(callee.TransferTarget(), callee.Task())\n", inner)
	fmt.Fprintf(w, "%sif err3_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err3_"), inner)
	fmt.Fprintf(w, "%s%s = uint64(dstH_.HandleID())\n", inner, slotExpr)
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLowerMemOwnWrite is the memory-write form of emitLowerFlatOwnWrite.
// Writes the resulting callee-side canon handle as a u32 to callee
// memory at ptrExpr.
func emitLowerMemOwnWrite(w *strings.Builder, rt *TypeResource, valExpr, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.HandleValueFor(caller, h, %s)\n", inner, rt.GoPackageQualifier, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	} else {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.bind(caller, h)\n", inner, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	}
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeOwn).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%ssrcH_, err2_ := caller.LookupOwn(tr_,callerH_)\n", inner)
	fmt.Fprintf(w, "%sif err2_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err2_"), inner)
	fmt.Fprintf(w, "%sdstH_, err3_ := srcH_.TransferOwn(callee.TransferTarget())\n", inner)
	fmt.Fprintf(w, "%sif err3_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err3_"), inner)
	fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, valExpr)
	fmt.Fprintf(w, "%s%s\n", inner, primMemWriteStmtErr(PrimU32, ptrExpr, "dstH_.HandleID()", "callee.Memory()", helperName, op+": handle", sink))
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLowerMemBorrowWrite is the memory-write form of
// emitLowerFlatBorrowWrite.
func emitLowerMemBorrowWrite(w *strings.Builder, rt *TypeResource, valExpr, ptrExpr, helperName, op string, sink errSink, indent, tyExpr string) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	if rt.IsImported() {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.HandleValueFor(caller, h, %s)\n", inner, rt.GoPackageQualifier, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	} else {
		fmt.Fprintf(w, "%scallerH_, err_ := %s.bind(caller, h)\n", inner, valExpr)
		fmt.Fprintf(w, "%sif err_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err_"), inner)
	}
	fmt.Fprintf(w, "%str_ := %s.(wacogo.TypeBorrow).ResourceType\n", inner, tyExpr)
	fmt.Fprintf(w, "%ssrcH_, err2_ := caller.LookupBorrowable(tr_,callerH_)\n", inner)
	fmt.Fprintf(w, "%sif err2_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err2_"), inner)
	fmt.Fprintf(w, "%sdstH_, err3_ := srcH_.LendTo(callee.TransferTarget(), callee.Task())\n", inner)
	fmt.Fprintf(w, "%sif err3_ != nil {\n%s\t%s\n%s}\n", inner, inner, sink.Emit("err3_"), inner)
	fmt.Fprintf(w, "%s%s\n", inner, primMemWriteStmtErr(PrimU32, ptrExpr, "dstH_.HandleID()", "callee.Memory()", helperName, op+": handle", sink))
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitLiftFlatOption emits the body for a *TypeOption in flat mode (lift).
func emitLiftFlatOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLift) {
	elemSlots := FlatSlots(v.Elem)
	fmt.Fprintf(w, "optT_ := ty.(wacogo.TypeOption)\n")
	fmt.Fprintf(w, "_ = optT_\n")
	fmt.Fprintf(w, "disc_ := uint32(stack[0])\n")
	fmt.Fprintf(w, "var out_ %s\n", GoTypeOf(v))
	fmt.Fprintf(w, "if disc_ != 0 {\n")
	fmt.Fprintf(w, "\tout_.IsSome = true\n")
	emitFlatSlotReadIntoLift(w, v.Elem, "out_.Value", 1, elemSlots, helperName, "Some payload", sink, "\t", "optT_.Inner")
	fmt.Fprintf(w, "}\n")
	fmt.Fprint(w, "return out_, nil\n")
}

// emitLiftFlatResult emits the body for a *TypeResult in flat mode using
// the sealed iface shape (<Name>Ok, <Name>Err case structs).
func emitLiftFlatResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLift) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	fmt.Fprintf(w, "resT_ := ty.(wacogo.TypeResult)\n")
	fmt.Fprintf(w, "_ = resT_\n")
	fmt.Fprintf(w, "disc_ := uint32(stack[0])\n")
	fmt.Fprintf(w, "if disc_ == 0 {\n")
	if v.OK != nil {
		okSlots := FlatSlots(v.OK)
		fmt.Fprintf(w, "\tvar okVal_ %s\n", GoTypeOf(v.OK))
		emitFlatSlotReadIntoLift(w, v.OK, "okVal_", 1, okSlots, helperName, "OK arm", sink, "\t", "resT_.Ok")
		fmt.Fprintf(w, "\treturn %s{Value: okVal_}, nil\n", okName)
	} else {
		fmt.Fprintf(w, "\treturn %s{}, nil\n", okName)
	}
	fmt.Fprintf(w, "}\n")
	if v.Err != nil {
		errSlots := FlatSlots(v.Err)
		fmt.Fprintf(w, "var errVal_ %s\n", GoTypeOf(v.Err))
		emitFlatSlotReadIntoLift(w, v.Err, "errVal_", 1, errSlots, helperName, "Err arm", sink, "", "resT_.Err")
		fmt.Fprintf(w, "return %s{Value: errVal_}, nil\n", errName)
	} else {
		fmt.Fprintf(w, "return %s{}, nil\n", errName)
	}
}

// emitLiftFlatVariant emits the body for a *TypeVariant in flat mode.
func emitLiftFlatVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLift) {
	fmt.Fprintf(w, "varT_ := ty.(wacogo.TypeVariant)\n")
	fmt.Fprintf(w, "_ = varT_\n")
	fmt.Fprint(w, "disc_ := uint32(stack[0])\n")
	fmt.Fprint(w, "switch disc_ {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %d:\n", i)
		if c.Payload == nil {
			fmt.Fprintf(w, "\treturn %s{}, nil\n", variantCaseName(v, c))
			continue
		}
		payloadSlots := FlatSlots(c.Payload)
		fmt.Fprintf(w, "\tvar val %s\n", GoTypeOf(c.Payload))
		emitFlatSlotReadIntoLift(w, c.Payload, "val", 1, payloadSlots, helperName, fmt.Sprintf("case %d payload", i), sink, "\t", fmt.Sprintf("varT_.Cases[%d].Payload", i))
		fmt.Fprintf(w, "\treturn %s{Value: val}, nil\n", variantCaseName(v, c))
	}
	fmt.Fprint(w, "}\n")
	fmt.Fprintf(w, "return %s, fmt.Errorf(\"wacogo/witgen: %s: invalid discriminant %%d\", disc_)\n", ZeroValueExpr(v), helperName)
}

// emitLiftMemOption is the memory-mode counterpart of emitLiftFlatOption.
func emitLiftMemOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLift) {
	payloadOffset := alignUp(1, Align(v.Elem))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "optT_ := ty.(wacogo.TypeOption)\n")
	fmt.Fprintf(w, "_ = optT_\n")
	fmt.Fprintf(w, "var disc_ uint8\n")
	fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU8, "disc_", "ptr", "callee.Memory()", helperName, "discriminant", sink))
	fmt.Fprintf(w, "var out_ %s\n", GoTypeOf(v))
	fmt.Fprintf(w, "if disc_ != 0 {\n")
	fmt.Fprintf(w, "\tout_.IsSome = true\n")
	emitElemReadIntoLift(w, v.Elem, "out_.Value", payloadPtr, helperName, "Some payload", sink, "\t", "optT_.Inner")
	fmt.Fprintf(w, "}\n")
	fmt.Fprint(w, "return out_, nil\n")
}

// emitLiftMemResult is the memory-mode counterpart of emitLiftFlatResult.
func emitLiftMemResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLift) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	payloadOffset := alignUp(1, resultPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "resT_ := ty.(wacogo.TypeResult)\n")
	fmt.Fprintf(w, "_ = resT_\n")
	fmt.Fprintf(w, "var disc_ uint8\n")
	fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU8, "disc_", "ptr", "callee.Memory()", helperName, "discriminant", sink))
	fmt.Fprintf(w, "if disc_ == 0 {\n")
	if v.OK != nil {
		fmt.Fprintf(w, "\tvar okVal_ %s\n", GoTypeOf(v.OK))
		emitElemReadIntoLift(w, v.OK, "okVal_", payloadPtr, helperName, "OK arm", sink, "\t", "resT_.Ok")
		fmt.Fprintf(w, "\treturn %s{Value: okVal_}, nil\n", okName)
	} else {
		fmt.Fprintf(w, "\treturn %s{}, nil\n", okName)
	}
	fmt.Fprintf(w, "}\n")
	if v.Err != nil {
		fmt.Fprintf(w, "var errVal_ %s\n", GoTypeOf(v.Err))
		emitElemReadIntoLift(w, v.Err, "errVal_", payloadPtr, helperName, "Err arm", sink, "", "resT_.Err")
		fmt.Fprintf(w, "return %s{Value: errVal_}, nil\n", errName)
	} else {
		fmt.Fprintf(w, "return %s{}, nil\n", errName)
	}
}

// emitLiftMemVariant is the memory-mode counterpart of emitLiftFlatVariant.
func emitLiftMemVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLift) {
	payloadOffset := alignUp(1, variantPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "varT_ := ty.(wacogo.TypeVariant)\n")
	fmt.Fprintf(w, "_ = varT_\n")
	fmt.Fprintf(w, "var disc_ uint8\n")
	fmt.Fprintf(w, "%s\n", primMemReadStmt(PrimU8, "disc_", "ptr", "callee.Memory()", helperName, "discriminant", sink))
	fmt.Fprint(w, "switch disc_ {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %d:\n", i)
		if c.Payload == nil {
			fmt.Fprintf(w, "\treturn %s{}, nil\n", variantCaseName(v, c))
			continue
		}
		fmt.Fprintf(w, "\tvar val %s\n", GoTypeOf(c.Payload))
		emitElemReadIntoLift(w, c.Payload, "val", payloadPtr, helperName, fmt.Sprintf("case %d payload", i), sink, "\t", fmt.Sprintf("varT_.Cases[%d].Payload", i))
		fmt.Fprintf(w, "\treturn %s{Value: val}, nil\n", variantCaseName(v, c))
	}
	fmt.Fprint(w, "}\n")
	fmt.Fprintf(w, "return %s, fmt.Errorf(\"wacogo/witgen: %s: invalid discriminant %%d\", disc_)\n", ZeroValueExpr(v), helperName)
}

// emitLowerFlatOption emits the body for a *TypeOption in flat encode mode (lower).
func emitLowerFlatOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLower) {
	elemSlots := FlatSlots(v.Elem)
	fmt.Fprintf(w, "optT_ := ty.(wacogo.TypeOption)\n")
	fmt.Fprintf(w, "_ = optT_\n")
	fmt.Fprintf(w, "if v.IsSome {\n")
	fmt.Fprintf(w, "\tstack[0] = 1\n")
	emitFlatSlotWriteFromLower(w, v.Elem, "v.Value", 1, elemSlots, helperName, "Some payload", sink, "\t", "optT_.Inner")
	fmt.Fprintf(w, "} else {\n")
	fmt.Fprintf(w, "\t_ = optT_\n")
	fmt.Fprintf(w, "\tstack[0] = 0\n")
	for i := 0; i < elemSlots; i++ {
		fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
	}
	fmt.Fprintf(w, "}\n")
}

// emitLowerFlatResult emits the body for a *TypeResult in flat encode
// mode using the sealed iface shape.
func emitLowerFlatResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLower) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	okSlots := 0
	if v.OK != nil {
		okSlots = FlatSlots(v.OK)
	}
	errSlots := 0
	if v.Err != nil {
		errSlots = FlatSlots(v.Err)
	}
	joinedSlots := okSlots
	if errSlots > joinedSlots {
		joinedSlots = errSlots
	}
	fmt.Fprintf(w, "resT_ := ty.(wacogo.TypeResult)\n")
	fmt.Fprintf(w, "_ = resT_\n")
	fmt.Fprintf(w, "switch x := v.(type) {\n")
	fmt.Fprintf(w, "case %s:\n", okName)
	fmt.Fprintf(w, "\tstack[0] = 0\n")
	if v.OK != nil {
		emitFlatSlotWriteFromLower(w, v.OK, "x.Value", 1, okSlots, helperName, "OK arm", sink, "\t", "resT_.Ok")
		for i := okSlots; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	} else {
		fmt.Fprint(w, "\t_ = x\n")
		for i := 0; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	}
	fmt.Fprintf(w, "case %s:\n", errName)
	fmt.Fprintf(w, "\tstack[0] = 1\n")
	if v.Err != nil {
		emitFlatSlotWriteFromLower(w, v.Err, "x.Value", 1, errSlots, helperName, "Err arm", sink, "\t", "resT_.Err")
		for i := errSlots; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	} else {
		fmt.Fprint(w, "\t_ = x\n")
		for i := 0; i < joinedSlots; i++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+i)
		}
	}
	fmt.Fprintf(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown result case")`, helperName)))
	fmt.Fprintf(w, "}\n")
}

// emitLowerFlatVariant emits the body for a *TypeVariant in flat encode mode.
func emitLowerFlatVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLower) {
	totalSlots := FlatSlots(v) - 1
	fmt.Fprintf(w, "varT_ := ty.(wacogo.TypeVariant)\n")
	fmt.Fprintf(w, "_ = varT_\n")
	fmt.Fprint(w, "switch x := v.(type) {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %s:\n", variantCaseName(v, c))
		fmt.Fprintf(w, "\tstack[0] = %d\n", i)
		payloadSlots := 0
		if c.Payload != nil {
			payloadSlots = FlatSlots(c.Payload)
			emitFlatSlotWriteFromLower(w, c.Payload, "x.Value", 1, payloadSlots, helperName, fmt.Sprintf("case %d payload", i), sink, "\t", fmt.Sprintf("varT_.Cases[%d].Payload", i))
		} else {
			fmt.Fprint(w, "\t_ = x\n")
		}
		for j := payloadSlots; j < totalSlots; j++ {
			fmt.Fprintf(w, "\tstack[%d] = 0\n", 1+j)
		}
	}
	fmt.Fprint(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown variant case")`, helperName)))
	fmt.Fprint(w, "}\n")
}

// emitLowerMemOption is the memory-mode counterpart of emitLowerFlatOption.
func emitLowerMemOption(w *strings.Builder, v *TypeOption, helperName string, sink errSinkLower) {
	payloadOffset := alignUp(1, Align(v.Elem))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "optT_ := ty.(wacogo.TypeOption)\n")
	fmt.Fprintf(w, "_ = optT_\n")
	fmt.Fprintf(w, "if v.IsSome {\n")
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "1", "callee.Memory()", helperName, "discriminant", sink))
	emitElemWriteFromLower(w, v.Elem, "v.Value", payloadPtr, helperName, "Some payload", sink, "\t", "optT_.Inner")
	fmt.Fprintf(w, "} else {\n")
	fmt.Fprintf(w, "\t_ = optT_\n")
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "0", "callee.Memory()", helperName, "discriminant", sink))
	fmt.Fprintf(w, "}\n")
}

// emitLowerMemResult is the memory-mode counterpart of emitLowerFlatResult.
func emitLowerMemResult(w *strings.Builder, v *TypeResult, helperName string, sink errSinkLower) {
	okName := ResultCaseGoName(v, "Ok")
	errName := ResultCaseGoName(v, "Err")
	payloadOffset := alignUp(1, resultPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "resT_ := ty.(wacogo.TypeResult)\n")
	fmt.Fprintf(w, "_ = resT_\n")
	fmt.Fprintf(w, "switch x := v.(type) {\n")
	fmt.Fprintf(w, "case %s:\n", okName)
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "0", "callee.Memory()", helperName, "discriminant", sink))
	if v.OK != nil {
		emitElemWriteFromLower(w, v.OK, "x.Value", payloadPtr, helperName, "OK arm", sink, "\t", "resT_.Ok")
	} else {
		fmt.Fprint(w, "\t_ = x\n")
	}
	fmt.Fprintf(w, "case %s:\n", errName)
	fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", "1", "callee.Memory()", helperName, "discriminant", sink))
	if v.Err != nil {
		emitElemWriteFromLower(w, v.Err, "x.Value", payloadPtr, helperName, "Err arm", sink, "\t", "resT_.Err")
	} else {
		fmt.Fprint(w, "\t_ = x\n")
	}
	fmt.Fprintf(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown result case")`, helperName)))
	fmt.Fprintf(w, "}\n")
}

// importedNominalAddTypeStmt returns the full Go statement that registers a
// cross-interface nominal type with the host.Builder. This is used by the
// factory template to declare typXxx variables for imported records, variants,
// enums, and flags that must be referenced in function signatures.
//
// The generated statement is of the form:
//
//	typ<GoName> := b.AddType("wit-name", host.Record{...})
func importedNominalAddTypeStmt(n ImportedNominal) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "typ%s_ := b_.AddType(%q, %s)\n\t_ = typ%s_", n.GoName, n.Name, nominalHostTypeDefExpr(n.Source), n.GoName)
	return sb.String()
}

// nominalHostTypeDefExpr returns the inline host.TypeExpr for the given
// nominal type — the second argument to b.AddType(). Unlike HostTypeExpr(),
// which returns a variable reference (typFoo), this always returns the
// structural definition (host.Record{...}, host.Variant{...}, etc.).
func nominalHostTypeDefExpr(t Type) string {
	switch v := t.(type) {
	case *TypeEnum:
		var sb strings.Builder
		sb.WriteString("host.Enum{Cases: []string{")
		for i, c := range v.Cases {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q", c.Name)
		}
		sb.WriteString("}}")
		return sb.String()
	case *TypeFlags:
		var sb strings.Builder
		sb.WriteString("host.Flags{Names: []string{")
		for i, c := range v.Cases {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q", c.Name)
		}
		sb.WriteString("}}")
		return sb.String()
	case *TypeRecord:
		var sb strings.Builder
		sb.WriteString("host.Record{Fields: []host.Field{")
		for i, f := range v.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "{Name: %q, Type: %s}", f.Name, hostTypeExpr(f.Type))
		}
		sb.WriteString("}}")
		return sb.String()
	case *TypeVariant:
		var sb strings.Builder
		sb.WriteString("host.Variant{Cases: []host.Case{")
		for i, c := range v.Cases {
			if i > 0 {
				sb.WriteString(", ")
			}
			payload := "nil"
			if c.Payload != nil {
				payload = hostTypeExpr(c.Payload)
			}
			fmt.Fprintf(&sb, "{Name: %q, Payload: %s}", c.Name, payload)
		}
		sb.WriteString("}}")
		return sb.String()
	}
	panic(fmt.Sprintf("witgen: nominalHostTypeDefExpr: unhandled %T", t))
}

// emitLowerMemVariant is the memory-mode counterpart of emitLowerFlatVariant.
func emitLowerMemVariant(w *strings.Builder, v *TypeVariant, helperName string, sink errSinkLower) {
	payloadOffset := alignUp(1, variantPayloadAlign(v))
	payloadPtr := "ptr"
	if payloadOffset > 0 {
		payloadPtr = fmt.Sprintf("ptr + %d", payloadOffset)
	}
	fmt.Fprintf(w, "varT_ := ty.(wacogo.TypeVariant)\n")
	fmt.Fprintf(w, "_ = varT_\n")
	fmt.Fprint(w, "switch x := v.(type) {\n")
	for i, c := range v.Cases {
		fmt.Fprintf(w, "case %s:\n", variantCaseName(v, c))
		fmt.Fprintf(w, "\t%s\n", primMemWriteStmtErr(PrimU8, "ptr", fmt.Sprintf("uint8(%d)", i), "callee.Memory()", helperName, "discriminant", sink))
		if c.Payload != nil {
			emitElemWriteFromLower(w, c.Payload, "x.Value", payloadPtr, helperName, fmt.Sprintf("case %d payload", i), sink, "\t", fmt.Sprintf("varT_.Cases[%d].Payload", i))
		} else {
			fmt.Fprint(w, "\t_ = x\n")
		}
	}
	fmt.Fprint(w, "default:\n")
	fmt.Fprintf(w, "\t%s\n", sink.Emit(fmt.Sprintf(`fmt.Errorf("wacogo/witgen: %s: unknown variant case")`, helperName)))
	fmt.Fprint(w, "}\n")
}
