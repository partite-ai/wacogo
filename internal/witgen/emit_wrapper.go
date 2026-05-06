package witgen

import (
	"bytes"
	"fmt"
	"strings"
)

// wrapperFunc describes one *host.ExportedFunc slot on the wrapper struct.
// FieldName is the unexported Go field on *<iface>Wrapper (e.g.
// "fnNewCounter"); WitExportName is the canonical WIT export name on
// the callee instance (e.g. "[constructor]counter"). One slot per
// callee-resolved exported function; matches the template's range.
type wrapperFunc struct {
	FieldName     string
	WitExportName string
}

// wrapperView is the template input for wrapper.tmpl.
type wrapperView struct {
	*Interface
	WrapName     string
	WrapperFuncs []wrapperFunc
	SourceFile   string
}

// emitWrapperFile produces the full text of <iface>.wrap.go for one
// Interface. The new (handle-management) shape:
//
//   - struct {{lcfirst .WrapName}}Wrapper { callerInst, fn<X> ... } where each
//     fn<X> is *host.ExportedFunc bound to the callee instance.
//   - WrapInstance(caller *host.ComponentInstance, callee *wacogo.ComponentInstance) {{.WrapName}}.
//   - method bodies use two-callback CallRaw: write closure runs
//     param lift on caller + lower on callee; read closure runs result
//     lift on callee + emit on caller. Resource-typed params/results
//     drive the *<R>Handle state machine via `bind` (or
//     `HandleValueFor` across package boundaries) and translate canon
//     handles between caller and callee tables via `IssueOwn` /
//     `ReleaseOwn` / `Lend` / `IssueBorrow` on the shared TR.
func emitWrapperFile(iface *Interface, sourceFile string) ([]byte, error) {
	var buf bytes.Buffer

	headerView := wrapperView{
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

	view := wrapperView{
		Interface:  iface,
		WrapName:   iface.WrapName,
		SourceFile: sourceFile,
	}

	// One ExportedFunc field per parent-level freestanding func.
	for _, f := range iface.Funcs {
		view.WrapperFuncs = append(view.WrapperFuncs, wrapperFunc{
			FieldName:     "fn" + f.GoName,
			WitExportName: f.WitName,
		})
	}

	// One ExportedFunc field per resource ctor / static / method on the
	// parent wrapper. (Per-resource forwarder dispatch tables for
	// imported resources are emitted into <iface>.bind.go and reached
	// via instanceState — they do not appear on the wrapper struct.)
	for _, rt := range iface.Resources {
		if rt.Ctor != nil {
			view.WrapperFuncs = append(view.WrapperFuncs, wrapperFunc{
				FieldName:     "fnNew" + rt.WrapName,
				WitExportName: "[constructor]" + rt.Name,
			})
		}
		for _, m := range rt.Methods {
			view.WrapperFuncs = append(view.WrapperFuncs, wrapperFunc{
				FieldName:     "fn" + rt.WrapName + m.GoName,
				WitExportName: "[method]" + rt.Name + "." + m.Name,
			})
		}
		for _, s := range rt.Statics {
			view.WrapperFuncs = append(view.WrapperFuncs, wrapperFunc{
				FieldName:     "fn" + s.GoName,
				WitExportName: "[static]" + rt.Name + "." + s.Name,
			})
		}
	}

	body, err := renderTemplate("wrapper.tmpl", view)
	if err != nil {
		return nil, err
	}
	buf.Write(body)

	// Build the set of imported package qualifiers that must not be used as
	// parameter names in concrete method bodies to avoid shadowing imports.
	reserved := make(map[string]bool)
	for _, imp := range iface.Imports {
		if imp.GoPackage != "" {
			reserved[imp.GoPackage] = true
		}
	}

	// Parent wrapper method bodies: freestanding funcs, then hoisted
	// ctors / statics for each resource.
	var wb strings.Builder
	for _, f := range iface.Funcs {
		emitParentWrapperFunc(&wb, iface.WrapName, f, reserved)
	}
	for _, rt := range iface.Resources {
		if rt.Ctor != nil {
			emitParentWrapperCtor(&wb, iface.WrapName, rt)
		}
		for _, s := range rt.Statics {
			emitParentWrapperStatic(&wb, iface.WrapName, rt, s, reserved)
		}
	}
	buf.WriteString(wb.String())

	return formatGoSource(iface.GoPackage+".wrap.go", buf.Bytes())
}

// ---- top-level method emitters ----

// emitParentWrapperFunc emits a *<iface>Wrapper method for a freestanding
// WIT func.
func emitParentWrapperFunc(w *strings.Builder, ifaceWrapName string, f *Func, reserved map[string]bool) {
	sig := buildMethodSignature(f.GoName, f.Params, f.Result, reserved)
	fmt.Fprintf(w, "func (w *%sWrapper) %s {\n", lcfirst(ifaceWrapName), sig)
	emitWrapBody(w, "w.fn"+f.GoName, "w.callerInst", "w", f.Params, f.Result, nil /* selfRT */, reserved)
	fmt.Fprint(w, "}\n\n")
}

// emitParentWrapperCtor emits a *<iface>Wrapper method for a hoisted
// resource ctor. The result is always own<R> lifted into a
// *<R>Handle via New<R>HandleFrom.
func emitParentWrapperCtor(w *strings.Builder, ifaceWrapName string, rt *TypeResource) {
	fmt.Fprintf(w, "func (w *%sWrapper) %s {\n",
		lcfirst(ifaceWrapName), ctorSigForSide(false, rt))
	// Reuse the generic emitter by treating the ctor's implicit own<R>
	// result as a regular result type.
	ownResult := &TypeOwn{Resource: rt}
	emitWrapBody(w, "w.fnNew"+rt.WrapName, "w.callerInst", "w", rt.Ctor.Params, ownResult, nil /* selfRT */)
	fmt.Fprint(w, "}\n\n")
}

// emitParentWrapperStatic emits a *<iface>Wrapper method for a hoisted
// resource static.
func emitParentWrapperStatic(w *strings.Builder, ifaceWrapName string, rt *TypeResource, s ResourceStatic, reserved map[string]bool) {
	sig := buildMethodSignature(s.GoName, s.Params, s.Result, reserved)
	fmt.Fprintf(w, "func (w *%sWrapper) %s {\n", lcfirst(ifaceWrapName), sig)
	emitWrapBody(w, "w.fn"+s.GoName, "w.callerInst", "w", s.Params, s.Result, nil /* selfRT */, reserved)
	fmt.Fprint(w, "}\n\n")
}

// buildMethodSignature returns "<Name>(<params>) <ret>" for the given
// Go method name, IR params, and IR result. Every emitted method
// returns either error (void result) or (T, error); for *TypeResult
// the inner T is the sealed interface type.
//
// reserved is an optional set of names that must not be used as parameter
// names in the concrete method body (e.g. imported package qualifiers that
// would shadow the import if used as a Go identifier). Colliding names are
// prefixed with "p_" to avoid the shadow.
func buildMethodSignature(goName string, params []*Param, result Type, reserved map[string]bool) string {
	ps := []string{"ctx context.Context"}
	for _, p := range params {
		name := p.GoName
		if reserved[name] {
			name = "p_" + name
		}
		ps = append(ps, name+" "+goTypeForSide(p.Type, false))
	}
	paramList := strings.Join(ps, ", ")
	return fmt.Sprintf("%s(%s) %s", goName, paramList, returnTypeForSide(result, false))
}

// ---- shared CallRaw body emitter ----

// emitWrapBody emits the body of a wrap.go method (after the opening "{"):
// result-var declarations, two-callback CallRaw invocation, error capture,
// and return statement.
//
//   - fnExpr: Go expression for the *host.ExportedFunc.
//   - callerInstExpr: Go expression for the caller-side
//     *host.ComponentInstance to pass to CallRaw.
//   - recv: receiver name on the surrounding method (e.g. "w" or "d").
//     Used to reference the receiver in implicit-self borrow handling
//     (e.g. `d.handle` for resource method receivers).
//   - params: WIT params (excludes the implicit borrow-self for methods).
//   - result: result type (nil for void).
//   - selfRT: when non-nil, this is the resource type whose receiver
//     supplies the implicit borrow<R> first arg. The receiver is
//     assumed to be `d.handle` (rep, same-component shortcut).
//
// User-supplied params from WIT can have any name (including `result`,
// `err`, `drop`, etc.) so inside the body we never reference them by
// their declared name — each param is cached as `argN` at the top, and
// the rest of the generated body lives in a nested `{ ... }` scope so
// any locally declared helper can shadow the original parameter name
// without conflict.
//
// Error handling: every helper invocation inside the CallRaw closures
// captures into a body-local `resErr_` and short-circuits the closure
// via early return. After CallRaw the body checks `callErr_` (CallRaw
// trap) and then `resErr_` (lift/lower failure) and returns the
// appropriate zero value plus error. A success path returns the
// captured `resVal_` along with nil.
func emitWrapBody(w *strings.Builder, fnExpr, callerInstExpr, recv string, params []*Param, result Type, selfRT *TypeResource, reserved ...map[string]bool) {
	emitWrapResultDecls(w, result)
	needsResErr := bodyWritesResErr(params, result)
	if needsResErr {
		fmt.Fprint(w, "\tvar resErr_ error\n")
	}

	// Build the reserved set from the optional parameter.
	res := map[string]bool{}
	if len(reserved) > 0 && reserved[0] != nil {
		res = reserved[0]
	}

	// Cache user params under collision-free names. If a param name collides
	// with an imported package qualifier (in reserved), the signature uses
	// p_<name>; use the same renamed form here.
	for i, p := range params {
		paramName := p.GoName
		if res[paramName] {
			paramName = "p_" + paramName
		}
		fmt.Fprintf(w, "\targ%d_ := %s\n", i, paramName)
	}

	// Open the body scope so generated locals are free to shadow
	// any user-chosen parameter name without conflict.
	fmt.Fprint(w, "\t{\n")

	// CallRaw header.
	fmt.Fprintf(w, "\t\tcallErr_ := %s.CallRaw(ctx, %s,\n", fnExpr, callerInstExpr)

	// write closure: caller-side lift of params, callee-side lower.
	fmt.Fprint(w, "\t\t\tfunc(caller, callee *host.CallContext, stack []uint64) {\n")
	off := 0
	paramBase := 0
	if selfRT != nil {
		// Self is borrow<R> of the receiver. Same-component shortcut:
		// the receiver carries the rep directly.
		fmt.Fprintf(w, "\t\t\t\tstack[0] = uint64(%s.handle)\n", recv)
		off = 1
		// Implicit-self occupies WIT param index 0; user params start at 1.
		paramBase = 1
	}
	for i, p := range params {
		emitWrapArgWrite(w, p, fmt.Sprintf("arg%d_", i), off, "\t\t\t\t", callerInstExpr, fnExpr, paramBase+i)
		off += FlatSlots(p.Type)
	}
	fmt.Fprint(w, "\t\t\t},\n")

	// read closure: callee-side lift of result.
	fmt.Fprint(w, "\t\t\tfunc(caller, callee *host.CallContext, stack []uint64) {\n")
	if result != nil {
		emitWrapResultRead(w, result, recv, callerInstExpr, fnExpr)
	}
	fmt.Fprint(w, "\t\t\t},\n")

	fmt.Fprint(w, "\t\t)\n")
	fmt.Fprintf(w, "\t\tif callErr_ != nil { %s }\n", returnWithErrExpr(result, "fmt.Errorf(\"wacogo/witgen: CallRaw failed: %w\", callErr_)"))
	if needsResErr {
		fmt.Fprintf(w, "\t\tif resErr_ != nil { %s }\n", returnWithErrExpr(result, "resErr_"))
	}

	fmt.Fprint(w, "\t}\n")

	emitWrapReturn(w, result)
}

// bodyWritesResErr reports whether the params lower + result lift will
// emit any closure-internal step that can fail and store into resErr_.
// Pure primitive/enum/flags traffic in both directions has no error
// path, so the resErr_ declaration and post-call check are dead and
// would trip "impossible condition" linters.
func bodyWritesResErr(params []*Param, result Type) bool {
	for _, p := range params {
		if typeWritesResErrLower(p.Type) {
			return true
		}
	}
	return typeWritesResErrLift(result)
}

// typeWritesResErrLower reports whether emitWrapArgWrite for t can
// store into resErr_. Mirrors the dispatch in emitWrapArgWrite: only
// Prim, *TypeEnum, and *TypeFlags lower with a single inline assignment
// and never fail.
func typeWritesResErrLower(t Type) bool {
	switch t.(type) {
	case Prim, *TypeEnum, *TypeFlags:
		return false
	}
	return true
}

// typeWritesResErrLift reports whether emitWrapResultRead for t can
// store into resErr_. nil result emits nothing; mem-mode results
// always go through liftMem<Name> which is fallible; otherwise only
// Prim, *TypeEnum, and *TypeFlags lift with a single inline assignment.
func typeWritesResErrLift(t Type) bool {
	if t == nil {
		return false
	}
	if FlatSlots(t) > 1 {
		return true
	}
	switch t.(type) {
	case Prim, *TypeEnum, *TypeFlags:
		return false
	}
	return true
}

// returnWithErrExpr emits a return statement that propagates errExpr
// in the proper return shape. For a nil result the method returns just
// error; otherwise it returns (zero, errExpr).
func returnWithErrExpr(result Type, errExpr string) string {
	if result == nil {
		return fmt.Sprintf("return %s", errExpr)
	}
	return fmt.Sprintf("return %s, %s", ZeroValueExpr(result), errExpr)
}

// emitWrapResultDecls emits the local variable declaration for the
// method's lifted result. For nil result no decl is emitted; otherwise
// `var resVal_ <GoTy>` is emitted, where <GoTy> is the user-facing type
// (sealed interface for *TypeResult, *<R>Handle for resources, etc.).
//
// The `res` prefix avoids collisions with user-supplied param names
// (`err`, `ok`, `result`) that may appear on the surrounding method.
func emitWrapResultDecls(w *strings.Builder, result Type) {
	if result == nil {
		return
	}
	fmt.Fprintf(w, "\tvar resVal_ %s\n", goTypeForSide(result, false))
}

// emitWrapArgWrite emits the param lift+lower stmt(s) inside the write
// closure. For primitives/enums/flags the lower is inline. For
// resource-typed args the spec's caller-lift / callee-lower pattern
// applies. For compound types we delegate to the lowerFlat<Name>
// helper, passing the callee CallContext (callee memory + realloc).
// On helper failure the closure stores into resErr_ and short-circuits.
//
// argName is the canonical body-local reference to the param value
// (e.g. "arg0"); the original WIT-derived name is reachable only via
// the function signature and is intentionally never used past the
// arg-caching prelude in emitWrapBody.
//
// fnExpr is the Go expression for the *host.ExportedFunc whose
// signature is being lowered; paramIdx is the WIT param index of p in
// that signature (accounting for any implicit-self at index 0). Used
// to source the callee-side resource type identity from the function
// being called rather than from a slot index in the caller's package.
func emitWrapArgWrite(w *strings.Builder, p *Param, argName string, slotOffset int, indent, callerInstExpr, fnExpr string, paramIdx int) {
	n := FlatSlots(p.Type)
	slotExpr := fmt.Sprintf("stack[%d]", slotOffset)
	switch ty := p.Type.(type) {
	case Prim:
		fmt.Fprintf(w, "%s%s = %s\n", indent, slotExpr, lowerFlatExpr(ty, argName))
	case *TypeEnum:
		fmt.Fprintf(w, "%s%s = %s\n", indent, slotExpr, lowerFlatEnumExpr(ty, argName))
	case *TypeFlags:
		fmt.Fprintf(w, "%s%s = %s\n", indent, slotExpr, lowerFlatFlagsExpr(ty, argName))
	case *TypeOwn:
		emitWrapOwnArgWrite(w, argName, ty.Resource, slotExpr, indent, callerInstExpr, fnExpr, paramIdx)
	case *TypeBorrow:
		emitWrapBorrowArgWrite(w, argName, ty.Resource, slotExpr, indent, callerInstExpr, fnExpr, paramIdx)
	default:
		// Compound flat lower goes through callee memory + callee realloc.
		fmt.Fprintf(w, "%sif err_ := lowerFlat%s(ctx, caller, callee, %s, %s.ParamType(%d), stack[%d:%d], %s); err_ != nil { resErr_ = err_; return }\n",
			indent, TypeName(p.Type), callerInstExpr, fnExpr, paramIdx, slotOffset, slotOffset+n, argName)
	}
}

// emitWrapOwnArgWrite emits the caller-side bind + callee-side issue
// chain for an own<R> param. arg.bind drives the *<R>Handle's state
// machine on the caller's table to obtain the canon handle; canon's
// transfer plan would normally translate that into the callee's
// handle space, but here the wrap.go path issues directly because
// the underlying TR is shared across both caller and callee canon
// tables (TR pointer identity is preserved by WithResourceFrom).
//
// After bind we Invalidate immediately: ownership of the canon entry
// transfers to the wasm call at the moment we put the handle on the
// stack, and the user can't observe the *<R>Handle between bind and
// the wasm call returning.
//
// argRef is the body-local expression that holds the *<R>Handle (e.g.
// "arg0"). The body is wrapped in a `{ ... }` block so the locally
// declared helpers (tr, callerH_, rep, err) cannot collide with later
// params handled in the same closure.
func emitWrapOwnArgWrite(w *strings.Builder, argRef string, rt *TypeResource, slotExpr, indent, callerInstExpr, fnExpr string, paramIdx int) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%str_ := %s.ParamType(%d).(wacogo.TypeOwn).ResourceType\n", inner, fnExpr, paramIdx)
	emitBindOrHandleValueFor(w, "callerH_", argRef, rt, inner, callerInstExpr)
	fmt.Fprintf(w, "%ssrcH_, err_ := caller.LookupOwn(tr_, callerH_)\n", inner)
	fmt.Fprintf(w, "%sif err_ != nil { resErr_ = err_; return }\n", inner)
	fmt.Fprintf(w, "%sdstH_, err_ := srcH_.TransferOwn(callee.TransferTarget())\n", inner)
	fmt.Fprintf(w, "%sif err_ != nil { resErr_ = err_; return }\n", inner)
	fmt.Fprintf(w, "%s%s = uint64(dstH_.HandleID())\n", inner, slotExpr)
	// Ownership transferred — invalidate the caller-side handle.
	fmt.Fprintf(w, "%s%s.Invalidate()\n", inner, argRef)
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitWrapBorrowArgWrite emits the rep + lend / issue-borrow chain
// for a borrow<R> param. arg.bind returns the canon handle on the
// caller's table; canon's transfer plan does Lend/Unlend bookkeeping
// atop the same TR. No Invalidate — the caller retains ownership.
func emitWrapBorrowArgWrite(w *strings.Builder, argRef string, rt *TypeResource, slotExpr, indent, callerInstExpr, fnExpr string, paramIdx int) {
	fmt.Fprintf(w, "%s{\n", indent)
	inner := indent + "\t"
	fmt.Fprintf(w, "%str_ := %s.ParamType(%d).(wacogo.TypeBorrow).ResourceType\n", inner, fnExpr, paramIdx)
	emitBindOrHandleValueFor(w, "callerH_", argRef, rt, inner, callerInstExpr)
	fmt.Fprintf(w, "%ssrcH_, err_ := caller.LookupBorrowable(tr_, callerH_)\n", inner)
	fmt.Fprintf(w, "%sif err_ != nil { resErr_ = err_; return }\n", inner)
	fmt.Fprintf(w, "%sdstH_, err_ := srcH_.LendTo(callee.TransferTarget(), callee.Task())\n", inner)
	fmt.Fprintf(w, "%sif err_ != nil { resErr_ = err_; return }\n", inner)
	fmt.Fprintf(w, "%s%s = uint64(dstH_.HandleID())\n", inner, slotExpr)
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitBindOrHandleValueFor writes the bind line that drives the
// handle's state machine to obtain its canon handle on the caller's
// table. Local R: arg.bind(caller, callerInstExpr). Foreign R:
// <pkg>.HandleValueFor(caller, callerInstExpr, arg) — the unexported
// bind method isn't reachable across package boundaries. Both forms
// produce a uint32 named outVar; on error the closure stores resErr_
// and short-circuits.
func emitBindOrHandleValueFor(w *strings.Builder, outVar, argRef string, rt *TypeResource, indent, callerInstExpr string) {
	if rt.IsImported() {
		fmt.Fprintf(w, "%s%s, err_ := %s.HandleValueFor(caller, %s, %s)\n",
			indent, outVar, rt.GoPackageQualifier, callerInstExpr, argRef)
	} else {
		fmt.Fprintf(w, "%s%s, err_ := %s.bind(caller, %s)\n", indent, outVar, argRef, callerInstExpr)
	}
	fmt.Fprintf(w, "%sif err_ != nil { resErr_ = err_; return }\n", indent)
}

// emitWrapResultRead emits the result lift inside the read closure.
// Mem-mode (FlatSlots > 1) goes through liftMem<Name> against callee
// memory; flat results lift directly off the stack. Helper failures
// store resErr_ and short-circuit. *TypeResult lifts directly to the
// sealed interface value — the inner business outcome flows out via
// resVal_ and the outer error is reserved for trap/infrastructure.
func emitWrapResultRead(w *strings.Builder, result Type, recv, callerInstExpr, fnExpr string) {
	if FlatSlots(result) > 1 {
		emitWrapResultReadMem(w, result, recv, callerInstExpr, fnExpr)
		return
	}
	switch ty := result.(type) {
	case Prim:
		fmt.Fprintf(w, "\t\t\tresVal_ = %s\n", liftFlatExpr(ty, "stack[0]"))
	case *TypeEnum:
		fmt.Fprintf(w, "\t\t\tresVal_ = %s\n", liftFlatEnumExpr(ty, "stack[0]"))
	case *TypeFlags:
		fmt.Fprintf(w, "\t\t\tresVal_ = %s\n", liftFlatFlagsExpr(ty, "stack[0]"))
	case *TypeOwn:
		emitWrapOwnResultRead(w, ty.Resource, "stack[0]", recv, callerInstExpr, fnExpr)
	default:
		// Compound flat result (including *TypeResult): liftFlat<Name>
		// against callee. liftFlat<Result*> already returns the sealed
		// interface, so we can assign directly to resVal_.
		n := FlatSlots(result)
		fmt.Fprintf(w, "\t\t\tif v_, err_ := liftFlat%s(ctx, caller, callee, %s, %s.ResultType(0), stack[0:%d]); err_ != nil { resErr_ = err_; return } else { resVal_ = v_ }\n", TypeName(result), callerInstExpr, fnExpr, n)
	}
}

// emitWrapResultReadMem emits the liftMem path for a result that doesn't
// fit in maxFlatResults slots — stack[0] is a pointer into callee memory.
// liftMem<Name> always returns the user-facing type directly (sealed
// interface for *TypeResult), so the result flows straight into resVal_.
func emitWrapResultReadMem(w *strings.Builder, result Type, recv, callerInstExpr, fnExpr string) {
	fmt.Fprintf(w, "\t\t\tif v_, err_ := liftMem%s(ctx, caller, callee, %s, %s.ResultType(0), uint32(stack[0])); err_ != nil { resErr_ = err_; return } else { resVal_ = v_ }\n", TypeName(result), callerInstExpr, fnExpr)
}

// emitWrapOwnResultRead emits the own-result lift: TransferOwn callee
// → caller, then construct *<R>Handle via New<R>HandleFrom. Two
// branches:
//   - Same-package R: dispatch impl is the local Go value found via
//     defining.LookupExtern on the rep. Direct dispatch — no forwarder.
//   - Cross-package R: dispatch impl is a forwarder wrapping
//     (caller, rh, fns), where fns lives on instanceState (precomputed
//     at NewInstance time).
func emitWrapOwnResultRead(w *strings.Builder, rt *TypeResource, slotExpr, recv, callerInstExpr, fnExpr string) {
	if rt.IsImported() {
		fmt.Fprint(w, "\t\t\t{\n")
		fmt.Fprintf(w, "\t\t\t\ttr_ := %s.ResultType(0).(wacogo.TypeOwn).ResourceType\n", fnExpr)
		fmt.Fprintf(w, "\t\t\t\tcalleeH_ := uint32(%s)\n", slotExpr)
		fmt.Fprint(w, "\t\t\t\tsrcH_, srcErr_ := callee.LookupOwn(tr_, calleeH_)\n")
		fmt.Fprint(w, "\t\t\t\tif srcErr_ != nil { resErr_ = srcErr_; return }\n")
		fmt.Fprint(w, "\t\t\t\tdstH_, dstErr_ := srcH_.TransferOwn(caller.TransferTarget())\n")
		fmt.Fprint(w, "\t\t\t\tif dstErr_ != nil { resErr_ = dstErr_; return }\n")
		fmt.Fprintf(w, "\t\t\t\tstate_ := %s.UserState().(*instanceState)\n", callerInstExpr)
		fmt.Fprintf(w, "\t\t\t\tfwd_ := &%s{caller: %s, rh: dstH_, fns: state_.%s}\n",
			forwarderTypeFor(rt), callerInstExpr, fwdFnsFieldFor(rt))
		fmt.Fprintf(w, "\t\t\t\tresVal_ = %s.New%sHandleFrom(dstH_, fwd_)\n",
			rt.GoPackageQualifier, rt.WrapName)
		fmt.Fprint(w, "\t\t\t}\n")
		return
	}
	fmt.Fprint(w, "\t\t\t{\n")
	fmt.Fprintf(w, "\t\t\t\ttr_ := %s.ResultType(0).(wacogo.TypeOwn).ResourceType\n", fnExpr)
	fmt.Fprintf(w, "\t\t\t\tcalleeH_ := uint32(%s)\n", slotExpr)
	fmt.Fprint(w, "\t\t\t\tsrcH_, srcErr_ := callee.LookupOwn(tr_, calleeH_)\n")
	fmt.Fprint(w, "\t\t\t\tif srcErr_ != nil { resErr_ = srcErr_; return }\n")
	fmt.Fprint(w, "\t\t\t\tdstH_, dstErr_ := srcH_.TransferOwn(caller.TransferTarget())\n")
	fmt.Fprint(w, "\t\t\t\tif dstErr_ != nil { resErr_ = dstErr_; return }\n")
	fmt.Fprintf(w, "\t\t\t\tvar impl_ %s\n", rt.WrapName)
	fmt.Fprintf(w, "\t\t\t\tif obj_, ok_ := dstH_.Type().Instance().LookupExtern(dstH_.Rep()); ok_ {\n")
	fmt.Fprintf(w, "\t\t\t\t\tif typed_, ok2_ := obj_.(%s); ok2_ { impl_ = typed_ }\n", rt.WrapName)
	fmt.Fprint(w, "\t\t\t\t}\n")
	fmt.Fprintf(w, "\t\t\t\tresVal_ = New%sHandleFrom(dstH_, impl_)\n", rt.WrapName)
	fmt.Fprint(w, "\t\t\t}\n")
}

// forwarderTypeFor returns the local Go type name of the importer
// package's forwarder for an imported resource. Looks up the value
// stashed on the original *TypeResource at lower-time via
// findImportedResourceFwd.
func forwarderTypeFor(rt *TypeResource) string {
	// rt is the per-iface shallow copy with GoPackageQualifier set.
	// The forwarder type name follows the convention used by lower.go.
	return rt.GoPackageQualifier + rt.WrapName + "Forwarder"
}

// fwdFnsFieldFor returns the unexported instanceState field holding
// the precomputed *<FwdFnsType> for an imported resource, following
// the lower.go convention.
func fwdFnsFieldFor(rt *TypeResource) string {
	return rt.GoPackageQualifier + rt.WrapName + "FwdFns"
}

// emitWrapReturn emits the success-path return statement for the
// result. Void result returns just nil; otherwise (resVal_, nil).
func emitWrapReturn(w *strings.Builder, result Type) {
	if result == nil {
		fmt.Fprint(w, "\treturn nil\n")
		return
	}
	fmt.Fprint(w, "\treturn resVal_, nil\n")
}
