package host

import (
	"fmt"

	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/partite-ai/wacogo/wasmparser"
)

// buildCoreFuncType produces a per-instance *core.FuncType by walking
// the user's *host.FuncType tree. Resource references (own<R>,
// borrow<R>) resolve via the trSlice (indexed by componentResourceTypeID
// on the *ResourceType / *ResourceTypeRef). Primitives and structural compounds
// unfold recursively.
func buildCoreFuncType(
	ft *FuncType,
	trSlice []*core.TypeResource,
) (*core.FuncType, error) {
	out := &core.FuncType{
		Params:  make([]core.ParamType, len(ft.Params)),
		Results: make([]core.ResultType, len(ft.Results)),
	}
	for i, p := range ft.Params {
		t, err := buildCoreType(p.Type, trSlice)
		if err != nil {
			return nil, fmt.Errorf("param %q: %w", p.Name, err)
		}
		out.Params[i] = core.ParamType{Name: p.Name, Type: t}
	}
	for i, r := range ft.Results {
		t, err := buildCoreType(r.Type, trSlice)
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		out.Results[i] = core.ResultType{Name: r.Name, Type: t}
	}
	return out, nil
}

// buildCoreType recursively walks a TypeExpr producing a per-instance
// core.Type. Resource refs on *TypeRef are resolved by looking up the
// componentResourceTypeID in trSlice. Regular *TypeRef (from AddType) recurses into
// the TypeRef's declExpr.
func buildCoreType(te TypeExpr, trSlice []*core.TypeResource) (core.Type, error) {
	switch v := te.(type) {
	case prim:
		return v.t, nil

	case *TypeRef:
		if v.ownOfResource != nil {
			tr := trSlice[v.ownOfResource.componentResourceTypeID]
			if tr == nil {
				return nil, fmt.Errorf("wacogo/host: own<R> refers to unpopulated TR slot %d", v.ownOfResource.componentResourceTypeID)
			}
			return core.TypeOwn{ResourceType: tr}, nil
		}
		if v.borrowOfResource != nil {
			tr := trSlice[v.borrowOfResource.componentResourceTypeID]
			if tr == nil {
				return nil, fmt.Errorf("wacogo/host: borrow<R> refers to unpopulated TR slot %d", v.borrowOfResource.componentResourceTypeID)
			}
			return core.TypeBorrow{ResourceType: tr}, nil
		}
		if v.ownOfResourceRef != nil {
			tr := trSlice[v.ownOfResourceRef.componentResourceTypeID]
			if tr == nil {
				return nil, fmt.Errorf("wacogo/host: own<R> for unresolved ResourceTypeRef %q", v.ownOfResourceRef.exportName)
			}
			return core.TypeOwn{ResourceType: tr}, nil
		}
		if v.borrowOfResourceRef != nil {
			tr := trSlice[v.borrowOfResourceRef.componentResourceTypeID]
			if tr == nil {
				return nil, fmt.Errorf("wacogo/host: borrow<R> for unresolved ResourceTypeRef %q", v.borrowOfResourceRef.exportName)
			}
			return core.TypeBorrow{ResourceType: tr}, nil
		}
		if v.declExpr == nil {
			return nil, fmt.Errorf("wacogo/host: *TypeRef has no declExpr (unresolved?)")
		}
		return buildCoreType(v.declExpr, trSlice)

	case List:
		elem, err := buildCoreType(v.Elem, trSlice)
		if err != nil {
			return nil, fmt.Errorf("list elem: %w", err)
		}
		return core.TypeList{Elem: elem}, nil

	case Tuple:
		types := make([]core.Type, len(v.Types))
		for i, te := range v.Types {
			t, err := buildCoreType(te, trSlice)
			if err != nil {
				return nil, fmt.Errorf("tuple[%d]: %w", i, err)
			}
			types[i] = t
		}
		return core.TypeTuple{Types: types}, nil

	case Option:
		inner, err := buildCoreType(v.Inner, trSlice)
		if err != nil {
			return nil, fmt.Errorf("option inner: %w", err)
		}
		return core.TypeOption{Inner: inner}, nil

	case Result:
		out := core.TypeResult{}
		if v.Ok != nil {
			ok, err := buildCoreType(v.Ok, trSlice)
			if err != nil {
				return nil, fmt.Errorf("result ok: %w", err)
			}
			out.Ok = ok
		}
		if v.Err != nil {
			er, err := buildCoreType(v.Err, trSlice)
			if err != nil {
				return nil, fmt.Errorf("result err: %w", err)
			}
			out.Err = er
		}
		return out, nil

	case Record:
		fields := make([]core.FieldType, len(v.Fields))
		for i, f := range v.Fields {
			t, err := buildCoreType(f.Type, trSlice)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", f.Name, err)
			}
			fields[i] = core.FieldType{Name: f.Name, Type: t}
		}
		return core.TypeRecord{Fields: fields}, nil

	case Variant:
		cases := make([]core.CaseType, len(v.Cases))
		for i, c := range v.Cases {
			cases[i] = core.CaseType{Name: c.Name}
			if c.Payload != nil {
				t, err := buildCoreType(c.Payload, trSlice)
				if err != nil {
					return nil, fmt.Errorf("case %q: %w", c.Name, err)
				}
				cases[i].Payload = t
			}
		}
		return core.TypeVariant{Cases: cases}, nil

	case Flags:
		return core.TypeFlags{Names: v.Names}, nil

	case Enum:
		return core.TypeEnum{Cases: v.Cases}, nil

	default:
		return nil, fmt.Errorf("wacogo/host: TypeExpr %T not supported", te)
	}
}

func primitiveOfCoreType(t core.Type) wasmparser.PrimitiveValType {
	switch t.(type) {
	case core.TypeBool:
		return wasmparser.PrimBool
	case core.TypeU8:
		return wasmparser.PrimU8
	case core.TypeU16:
		return wasmparser.PrimU16
	case core.TypeU32:
		return wasmparser.PrimU32
	case core.TypeU64:
		return wasmparser.PrimU64
	case core.TypeS8:
		return wasmparser.PrimS8
	case core.TypeS16:
		return wasmparser.PrimS16
	case core.TypeS32:
		return wasmparser.PrimS32
	case core.TypeS64:
		return wasmparser.PrimS64
	case core.TypeF32:
		return wasmparser.PrimF32
	case core.TypeF64:
		return wasmparser.PrimF64
	case core.TypeChar:
		return wasmparser.PrimChar
	case core.TypeString:
		return wasmparser.PrimString
	default:
		panic(fmt.Sprintf("wacogo/host: primitiveOfCoreType: non-primitive %T", t))
	}
}

const (
	maxFlatParams  = 16
	maxFlatResults = 1
)

// validateFuncType checks structural rules at Build time without
// pushing arena entries: nominal compounds must not appear inline,
// and *TypeRef from AddType must carry a declExpr.
func validateFuncType(ft *FuncType) error {
	for _, p := range ft.Params {
		if err := validateValTypeExpr(p.Type); err != nil {
			return fmt.Errorf("param %q: %w", p.Name, err)
		}
	}
	for _, r := range ft.Results {
		if err := validateValTypeExpr(r.Type); err != nil {
			return fmt.Errorf("result: %w", err)
		}
	}
	return nil
}

// validateTopLevelTypeExpr admits nominal compounds (Record/Variant/
// Flags/Enum) at the top level (the only legal position) and recurses
// into their children with validateValTypeExpr.
func validateTopLevelTypeExpr(te TypeExpr) error {
	switch v := te.(type) {
	case prim:
		return fmt.Errorf("cannot register primitive via AddType (use primitives inline)")
	case Record:
		for _, f := range v.Fields {
			if err := validateValTypeExpr(f.Type); err != nil {
				return fmt.Errorf("field %q: %w", f.Name, err)
			}
		}
		return nil
	case Variant:
		for _, c := range v.Cases {
			if c.Payload == nil {
				continue
			}
			if err := validateValTypeExpr(c.Payload); err != nil {
				return fmt.Errorf("case %q: %w", c.Name, err)
			}
		}
		return nil
	case Flags, Enum:
		return nil
	default:
		return validateValTypeExpr(te)
	}
}

// validateValTypeExpr enforces inline-position rules: nominal compounds
// are rejected, structural compounds recurse, *TypeRef must be resolved.
func validateValTypeExpr(te TypeExpr) error {
	switch v := te.(type) {
	case prim:
		return nil
	case *TypeRef:
		if v.ownOfResource != nil || v.borrowOfResource != nil ||
			v.ownOfResourceRef != nil || v.borrowOfResourceRef != nil {
			return nil
		}
		if v.declExpr == nil {
			return fmt.Errorf("*TypeRef is unresolved (not registered via AddType?)")
		}
		return nil
	case List:
		return validateValTypeExpr(v.Elem)
	case Tuple:
		for i, te := range v.Types {
			if err := validateValTypeExpr(te); err != nil {
				return fmt.Errorf("tuple[%d]: %w", i, err)
			}
		}
		return nil
	case Option:
		return validateValTypeExpr(v.Inner)
	case Result:
		if v.Ok != nil {
			if err := validateValTypeExpr(v.Ok); err != nil {
				return fmt.Errorf("result ok: %w", err)
			}
		}
		if v.Err != nil {
			if err := validateValTypeExpr(v.Err); err != nil {
				return fmt.Errorf("result err: %w", err)
			}
		}
		return nil
	case Record:
		return fmt.Errorf("Record must be registered via AddType; inline use not permitted")
	case Variant:
		return fmt.Errorf("Variant must be registered via AddType; inline use not permitted")
	case Flags:
		return fmt.Errorf("Flags must be registered via AddType; inline use not permitted")
	case Enum:
		return fmt.Errorf("Enum must be registered via AddType; inline use not permitted")
	default:
		return fmt.Errorf("TypeExpr %T not supported", te)
	}
}

// flattenFuncForStub computes the canonical-ABI flat signature of the
// stub wrapper for a *host.FuncType. Flatten shape is
// resource-identity-independent (own/borrow both flatten to i32), so
// this runs at Build time directly against the user-supplied TypeExpr
// tree.
func flattenFuncForStub(ft *FuncType) (params, results []byte, err error) {
	paramCount := uint32(0)
	for _, p := range ft.Params {
		n, err := flatCountTypeExpr(p.Type)
		if err != nil {
			return nil, nil, err
		}
		paramCount += n
	}
	resultCount := uint32(0)
	for _, r := range ft.Results {
		n, err := flatCountTypeExpr(r.Type)
		if err != nil {
			return nil, nil, err
		}
		resultCount += n
	}

	if paramCount <= maxFlatParams {
		for _, p := range ft.Params {
			cvs, err := flattenTypeExprBytes(p.Type)
			if err != nil {
				return nil, nil, err
			}
			params = append(params, cvs...)
		}
	} else {
		params = []byte{wasm.ValI32}
	}

	if resultCount <= maxFlatResults {
		for _, r := range ft.Results {
			cvs, err := flattenTypeExprBytes(r.Type)
			if err != nil {
				return nil, nil, err
			}
			results = append(results, cvs...)
		}
	} else {
		// canon.lift convention: callee allocates and returns a single
		// i32 pointer. The host Go fn is responsible for writing its
		// results into that buffer before returning the pointer.
		results = []byte{wasm.ValI32}
	}

	return params, results, nil
}

// flatCountTypeExpr returns the flat slot count for a TypeExpr (what
// canon.FlatCount would report for the corresponding core.Type).
func flatCountTypeExpr(te TypeExpr) (uint32, error) {
	b, err := flattenTypeExprBytes(te)
	if err != nil {
		return 0, err
	}
	return uint32(len(b)), nil
}

// flattenTypeExprBytes returns the internal/wasm.Val* byte
// representation of a TypeExpr's flat slots. Mirrors the core-level
// flatten but walks user-supplied TypeExprs so we never need to
// materialise a *core.FuncType at Build time.
func flattenTypeExprBytes(te TypeExpr) ([]byte, error) {
	switch v := te.(type) {
	case prim:
		return flattenPrimBytes(v.t)
	case *TypeRef:
		if v.ownOfResource != nil || v.borrowOfResource != nil ||
			v.ownOfResourceRef != nil || v.borrowOfResourceRef != nil {
			return []byte{wasm.ValI32}, nil
		}
		if v.declExpr == nil {
			return nil, fmt.Errorf("wacogo/host: flatten: *TypeRef has no declExpr")
		}
		return flattenTypeExprBytes(v.declExpr)
	case List:
		return []byte{wasm.ValI32, wasm.ValI32}, nil
	case Tuple:
		var out []byte
		for _, e := range v.Types {
			cvs, err := flattenTypeExprBytes(e)
			if err != nil {
				return nil, err
			}
			out = append(out, cvs...)
		}
		return out, nil
	case Option:
		inner, err := flattenTypeExprBytes(v.Inner)
		if err != nil {
			return nil, err
		}
		return append([]byte{wasm.ValI32}, inner...), nil
	case Result:
		var okF, errF []byte
		var err error
		if v.Ok != nil {
			okF, err = flattenTypeExprBytes(v.Ok)
			if err != nil {
				return nil, err
			}
		}
		if v.Err != nil {
			errF, err = flattenTypeExprBytes(v.Err)
			if err != nil {
				return nil, err
			}
		}
		joined := joinFlatTypes(okF, errF)
		return append([]byte{wasm.ValI32}, joined...), nil
	case Record:
		var out []byte
		for _, f := range v.Fields {
			cvs, err := flattenTypeExprBytes(f.Type)
			if err != nil {
				return nil, err
			}
			out = append(out, cvs...)
		}
		return out, nil
	case Variant:
		var joined []byte
		for _, c := range v.Cases {
			if c.Payload == nil {
				continue
			}
			payload, err := flattenTypeExprBytes(c.Payload)
			if err != nil {
				return nil, err
			}
			joined = joinFlatTypes(joined, payload)
		}
		return append([]byte{wasm.ValI32}, joined...), nil
	case Flags:
		n := (len(v.Names) + 31) / 32
		if n < 1 {
			n = 1
		}
		out := make([]byte, n)
		for i := range out {
			out[i] = wasm.ValI32
		}
		return out, nil
	case Enum:
		return []byte{wasm.ValI32}, nil
	default:
		return nil, fmt.Errorf("wacogo/host: flatten: unsupported TypeExpr %T", te)
	}
}

// flattenPrimBytes returns the flat slot bytes for a primitive
// core.Type (as carried by a TypeExpr of kind prim).
func flattenPrimBytes(t core.Type) ([]byte, error) {
	switch t.(type) {
	case core.TypeBool,
		core.TypeU8, core.TypeU16, core.TypeU32,
		core.TypeS8, core.TypeS16, core.TypeS32,
		core.TypeChar:
		return []byte{wasm.ValI32}, nil
	case core.TypeU64, core.TypeS64:
		return []byte{wasm.ValI64}, nil
	case core.TypeF32:
		return []byte{wasm.ValF32}, nil
	case core.TypeF64:
		return []byte{wasm.ValF64}, nil
	case core.TypeString:
		return []byte{wasm.ValI32, wasm.ValI32}, nil
	default:
		return nil, fmt.Errorf("wacogo/host: flatten: unsupported primitive %T", t)
	}
}

// joinFlatTypes computes the element-wise join of two flat-type slices,
// matching the canonical-ABI variant-payload join rule. Extends the
// shorter to the longer and joins each position via the lattice:
//
//	i32 join i32 = i32
//	i32 join i64 = i64
//	i32 join f32 = i32
//	i32 join f64 = i64
//	f32 join f32 = f32
//	f32 join f64 = f64
//	f64 join f64 = f64
//	(any other mixed) = i64
func joinFlatTypes(a, b []byte) []byte {
	out := make([]byte, max(len(a), len(b)))
	for i := range out {
		switch {
		case i >= len(a):
			out[i] = b[i]
		case i >= len(b):
			out[i] = a[i]
		default:
			out[i] = joinFlat(a[i], b[i])
		}
	}
	return out
}

func joinFlat(a, b byte) byte {
	if a == b {
		return a
	}
	// Mixed int/float always widens to i64.
	return wasm.ValI64
}

// instTranslator carries per-Instantiate arena state. Each
// host.Component.Instantiate constructs one of these, populates
// resResID with freshly-allocated arena ResourceIDs for every declared
// resource and refResID with each ResourceTypeRef's lender-supplied
// ResourceID, then translates every func signature into the engine's
// arena. The resulting func and ComponentType IDs are tied to that
// specific instance's resource identity — placeholder IDs from Build
// are never reused.
//
// Build-time *TypeRef.hasArena / *ResourceType.resID /
// *ResourceTypeRef.resID fields are NOT consulted by this translator.
// The Build-time push path remains intact for the time being but its
// arena entries are unused at Instantiate.
type instTranslator struct {
	htb          *wasmparser.HostTypeBuilder
	typeRefArena map[*TypeRef]uint32
	resResID     map[*ResourceType]wasmparser.ResourceID
	refResID     map[*ResourceTypeRef]wasmparser.ResourceID
}

func newInstTranslator(htb *wasmparser.HostTypeBuilder) *instTranslator {
	return &instTranslator{
		htb:          htb,
		typeRefArena: make(map[*TypeRef]uint32),
		resResID:     make(map[*ResourceType]wasmparser.ResourceID),
		refResID:     make(map[*ResourceTypeRef]wasmparser.ResourceID),
	}
}

// instTranslateValType is the per-instance counterpart of
// translateValType: it pushes arena entries against t.htb and resolves
// resource handles via t.resResID / t.refResID rather than the
// Build-time fields on *ResourceType / *ResourceTypeRef. Memoization is
// scoped to t.typeRefArena, so a *TypeRef used in two signatures of
// the same instance shares one DefinedTypeDesc but does not collide
// with a sibling instance's memoization.
func (t *instTranslator) instTranslateValType(te TypeExpr) (wasmparser.ValTypeDesc, error) {
	switch v := te.(type) {
	case prim:
		return wasmparser.ValTypeDesc{
			IsPrimitive: true,
			Primitive:   primitiveOfCoreType(v.t),
		}, nil

	case *TypeRef:
		if id, ok := t.typeRefArena[v]; ok {
			return wasmparser.ValTypeDesc{TypeID: wasmparser.ComponentDefinedTypeID(id)}, nil
		}
		switch {
		case v.ownOfResource != nil:
			rid, ok := t.resResID[v.ownOfResource]
			if !ok {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: own<R> for unregistered ResourceType %q", v.ownOfResource.exportName)
			}
			id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindOwn, Own: rid})
			t.typeRefArena[v] = uint32(id)
			return wasmparser.ValTypeDesc{TypeID: id}, nil
		case v.borrowOfResource != nil:
			rid, ok := t.resResID[v.borrowOfResource]
			if !ok {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: borrow<R> for unregistered ResourceType %q", v.borrowOfResource.exportName)
			}
			id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindBorrow, Borrow: rid})
			t.typeRefArena[v] = uint32(id)
			return wasmparser.ValTypeDesc{TypeID: id}, nil
		case v.ownOfResourceRef != nil:
			rid, ok := t.refResID[v.ownOfResourceRef]
			if !ok {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: own<R> for ResourceTypeRef %q has no lender binding", v.ownOfResourceRef.exportName)
			}
			id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindOwn, Own: rid})
			t.typeRefArena[v] = uint32(id)
			return wasmparser.ValTypeDesc{TypeID: id}, nil
		case v.borrowOfResourceRef != nil:
			rid, ok := t.refResID[v.borrowOfResourceRef]
			if !ok {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: borrow<R> for ResourceTypeRef %q has no lender binding", v.borrowOfResourceRef.exportName)
			}
			id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindBorrow, Borrow: rid})
			t.typeRefArena[v] = uint32(id)
			return wasmparser.ValTypeDesc{TypeID: id}, nil
		case v.declExpr != nil:
			vt, err := t.instTranslateTopLevel(v.declExpr)
			if err != nil {
				return wasmparser.ValTypeDesc{}, err
			}
			if !vt.IsPrimitive {
				t.typeRefArena[v] = uint32(vt.TypeID)
			}
			return vt, nil
		default:
			return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: *TypeRef is unresolved")
		}

	case List:
		elemVT, err := t.instTranslateValType(v.Elem)
		if err != nil {
			return wasmparser.ValTypeDesc{}, fmt.Errorf("list elem: %w", err)
		}
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindList, List: elemVT})
		return wasmparser.ValTypeDesc{TypeID: id}, nil

	case Tuple:
		vts := make([]wasmparser.ValTypeDesc, len(v.Types))
		for i, te := range v.Types {
			vt, err := t.instTranslateValType(te)
			if err != nil {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("tuple[%d]: %w", i, err)
			}
			vts[i] = vt
		}
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindTuple, Tuple: vts})
		return wasmparser.ValTypeDesc{TypeID: id}, nil

	case Option:
		innerVT, err := t.instTranslateValType(v.Inner)
		if err != nil {
			return wasmparser.ValTypeDesc{}, fmt.Errorf("option inner: %w", err)
		}
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{Kind: wasmparser.DefinedKindOption, Option: innerVT})
		return wasmparser.ValTypeDesc{TypeID: id}, nil

	case Result:
		var okVT, errVT wasmparser.Optional[wasmparser.ValTypeDesc]
		if v.Ok != nil {
			vt, err := t.instTranslateValType(v.Ok)
			if err != nil {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("result ok: %w", err)
			}
			okVT = wasmparser.Some(vt)
		}
		if v.Err != nil {
			vt, err := t.instTranslateValType(v.Err)
			if err != nil {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("result err: %w", err)
			}
			errVT = wasmparser.Some(vt)
		}
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{
			Kind: wasmparser.DefinedKindResult, ResultOk: okVT, ResultErr: errVT,
		})
		return wasmparser.ValTypeDesc{TypeID: id}, nil

	case Record:
		return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: Record must be registered via AddType; inline use not permitted")
	case Variant:
		return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: Variant must be registered via AddType; inline use not permitted")
	case Flags:
		return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: Flags must be registered via AddType; inline use not permitted")
	case Enum:
		return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: Enum must be registered via AddType; inline use not permitted")

	default:
		return wasmparser.ValTypeDesc{}, fmt.Errorf("wacogo/host: TypeExpr %T not supported", te)
	}
}

func (t *instTranslator) instTranslateTopLevel(te TypeExpr) (wasmparser.ValTypeDesc, error) {
	switch v := te.(type) {
	case Record:
		fields := make([]wasmparser.FieldDesc, len(v.Fields))
		for i, f := range v.Fields {
			vt, err := t.instTranslateValType(f.Type)
			if err != nil {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("field %q: %w", f.Name, err)
			}
			fields[i] = wasmparser.FieldDesc{Name: f.Name, Type: vt}
		}
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{
			Kind:   wasmparser.DefinedKindRecord,
			Record: &wasmparser.RecordTypeDesc{Fields: fields},
		})
		return wasmparser.ValTypeDesc{TypeID: id}, nil
	case Variant:
		cases := make([]wasmparser.VariantCaseDesc, len(v.Cases))
		for i, c := range v.Cases {
			if c.Payload == nil {
				cases[i] = wasmparser.VariantCaseDesc{Name: c.Name}
				continue
			}
			vt, err := t.instTranslateValType(c.Payload)
			if err != nil {
				return wasmparser.ValTypeDesc{}, fmt.Errorf("case %q: %w", c.Name, err)
			}
			cases[i] = wasmparser.VariantCaseDesc{Name: c.Name, Type: wasmparser.Some(vt)}
		}
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{
			Kind:    wasmparser.DefinedKindVariant,
			Variant: &wasmparser.VariantTypeDesc{Cases: cases},
		})
		return wasmparser.ValTypeDesc{TypeID: id}, nil
	case Flags:
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{
			Kind:  wasmparser.DefinedKindFlags,
			Flags: v.Names,
		})
		return wasmparser.ValTypeDesc{TypeID: id}, nil
	case Enum:
		id := t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{
			Kind: wasmparser.DefinedKindEnum,
			Enum: v.Cases,
		})
		return wasmparser.ValTypeDesc{TypeID: id}, nil
	default:
		return t.instTranslateValType(te)
	}
}

// instTranslateDefinedType translates a registered *TypeRef and returns
// the arena ID of the resulting defined type, suitable for an
// AnyTypeDefined export entry. A *TypeRef whose declaration collapses to
// a bare primitive is wrapped in a primitive alias so it still has an ID.
func (t *instTranslator) instTranslateDefinedType(ref *TypeRef) (wasmparser.ComponentDefinedTypeID, error) {
	vt, err := t.instTranslateValType(ref)
	if err != nil {
		return 0, err
	}
	if vt.IsPrimitive {
		return t.htb.PushDefinedType(wasmparser.DefinedTypeDesc{
			Kind: wasmparser.DefinedKindPrimitive, Primitive: vt.Primitive,
		}), nil
	}
	return vt.TypeID, nil
}

func (t *instTranslator) instTranslateFuncType(ft *FuncType) (wasmparser.FuncTypeDesc, error) {
	fd := wasmparser.FuncTypeDesc{}
	for _, p := range ft.Params {
		vt, err := t.instTranslateValType(p.Type)
		if err != nil {
			return wasmparser.FuncTypeDesc{}, fmt.Errorf("param %q: %w", p.Name, err)
		}
		fd.Params = append(fd.Params, wasmparser.FuncParamDesc{Name: p.Name, Type: vt})
	}
	for _, r := range ft.Results {
		vt, err := t.instTranslateValType(r.Type)
		if err != nil {
			return wasmparser.FuncTypeDesc{}, fmt.Errorf("result: %w", err)
		}
		fd.Results = append(fd.Results, wasmparser.FuncParamDesc{Name: r.Name, Type: vt})
	}
	return fd, nil
}
