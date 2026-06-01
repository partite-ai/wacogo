package core

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
)

// This file bridges parent-package types to the canon package's interfaces.
// resourceType wraps *TypeResource to satisfy canon.ResourceType without
// polluting the public *TypeResource API with canon-specific methods.
// canonResourceTableView wraps *ResourceTable to satisfy canon.ResourceTable,
// so *ResourceTable's public methods can speak *TypeResource directly.

// canonInstanceView wraps *ComponentInstance to satisfy canon.Instance.
// The wrapper exists so *ComponentInstance's public API does not carry a
// ResourceTable accessor — external holders of a *ComponentInstance cannot
// reach the resource table; canon (internal-only) reaches it through this
// adapter.
type canonInstanceView struct{ i *ComponentInstance }

func (v canonInstanceView) Enter(ctx context.Context) error { return v.i.Enter(ctx) }
func (v canonInstanceView) Exit(ctx context.Context)        { v.i.Exit(ctx) }
func (v canonInstanceView) CanLeave() bool                  { return v.i.CanLeave() }
func (v canonInstanceView) SuspendLeave() bool              { return v.i.SuspendLeave() }
func (v canonInstanceView) RestoreLeave(prev bool)          { v.i.RestoreLeave(prev) }
func (v canonInstanceView) Poison(reason error)             { v.i.Poison(reason) }
func (v canonInstanceView) ResourceTable() canon.ResourceTable {
	if v.i == nil || v.i.resources == nil {
		return nil
	}
	return canonResourceTableView{t: v.i.resources}
}

var _ canon.Instance = canonInstanceView{}

// InstanceAsCanon returns i as a canon.Instance. The returned value wraps i
// in a canonInstanceView so that *ComponentInstance's public API does not
// expose a ResourceTable accessor; canon reaches the table only through this
// adapter. Use this when building canon.CallSide or canon.Callee values from
// external packages (e.g. host).
func InstanceAsCanon(i *ComponentInstance) canon.Instance {
	return canonInstanceView{i: i}
}

// resourceType adapts *TypeResource to satisfy canon.ResourceType.
type resourceType struct{ rt *TypeResource }

func (r resourceType) IsResourceType() {}

func (r resourceType) DefiningInstance() canon.Instance {
	if r.rt.instance == nil {
		return nil
	}
	return canonInstanceView{i: r.rt.instance}
}

func (r resourceType) Destructor() func(ctx context.Context, rep uint32) error {
	if r.rt == nil {
		return nil
	}
	return r.rt.dtor
}

// canonResourceTableView wraps *ResourceTable to satisfy canon.ResourceTable.
// canon visitors receive this view and pass canon.ResourceType values (wrapped
// via resourceType{}); the view unwraps them back to *TypeResource before
// forwarding to the underlying table.
type canonResourceTableView struct{ t *ResourceTable }

func (v canonResourceTableView) IssueOwn(rt canon.ResourceType, rep uint32) canon.ResourceHandle {
	tr := rt.(resourceType).rt
	return canonResourceHandleView{h: v.t.IssueOwn(tr, rep)}
}

func (v canonResourceTableView) LookupOwn(rt canon.ResourceType, h uint32) (canon.ResourceHandle, error) {
	tr := rt.(resourceType).rt
	handle, err := v.t.LookupOwn(tr, h)
	if err != nil {
		return nil, err
	}
	return canonResourceHandleView{h: handle}, nil
}

func (v canonResourceTableView) LookupBorrowable(rt canon.ResourceType, h uint32) (canon.ResourceHandle, error) {
	tr := rt.(resourceType).rt
	handle, err := v.t.LookupBorrowable(tr, h)
	if err != nil {
		return nil, err
	}
	return canonResourceHandleView{h: handle}, nil
}

func (v canonResourceTableView) IssueBorrow(rt canon.ResourceType, rep uint32, task *canon.Task) canon.ResourceHandle {
	tr := rt.(resourceType).rt
	return canonResourceHandleView{h: v.t.issueBorrow(tr, rep, task)}
}

func (v canonResourceTableView) Owner() canon.Instance {
	if v.t == nil || v.t.owner == nil {
		return nil
	}
	return canonInstanceView{i: v.t.owner}
}

// Interface satisfaction assertion — the view satisfies the canon interface,
// not *ResourceTable directly.
var _ canon.ResourceTable = canonResourceTableView{}

// canonResourceHandleView wraps a core.ResourceHandle to satisfy the
// canon.ResourceHandle interface. canon visitors interact through this
// view; the view forwards to the underlying core handle, unwrapping
// canon.ResourceTable arguments back to *ResourceTable.
type canonResourceHandleView struct{ h ResourceHandle }

func (v canonResourceHandleView) HandleID() uint32 { return v.h.HandleID() }
func (v canonResourceHandleView) Rep() uint32      { return v.h.Rep() }

func (v canonResourceHandleView) Type() canon.ResourceType {
	tr := v.h.Type()
	if tr == nil {
		return nil
	}
	return resourceType{rt: tr}
}

func (v canonResourceHandleView) LendTo(target canon.ResourceTable, task *canon.Task) (canon.ResourceHandle, error) {
	var coreTarget TransferTarget
	if target != nil {
		coreTarget = unwrapCanonTable(target)
	}
	res, err := v.h.LendTo(coreTarget, task)
	if err != nil {
		return nil, err
	}
	return canonResourceHandleView{h: res}, nil
}

func (v canonResourceHandleView) TransferOwn(target canon.TransferTarget) (canon.ResourceHandle, error) {
	if target == nil {
		return nil, fmt.Errorf("TransferOwn: nil target")
	}
	// Fast path: target is a canon view of a *core.ResourceTable. Pass
	// the underlying *ResourceTable straight through.
	if tbl, ok := target.(canonResourceTableView); ok {
		res, err := v.h.TransferOwn(tbl.t)
		if err != nil {
			return nil, err
		}
		return canonResourceHandleView{h: res}, nil
	}
	// Non-table target (e.g. *canon.ValOwnHandle): wrap as a core
	// TransferTarget shim. liveResourceHandle.TransferOwn does its
	// normal slot-free + IssueOwn dispatch, the shim forwards IssueOwn
	// across the canon boundary, and the held canon.ResourceHandle is
	// unwrapped from the holder for the final return.
	shim := canonTargetShim{ct: target}
	res, err := v.h.TransferOwn(shim)
	if err != nil {
		return nil, err
	}
	if holder, ok := res.(holderHandle); ok {
		return holder.canonH, nil
	}
	return canonResourceHandleView{h: res}, nil
}

// canonTargetShim wraps a canon.TransferTarget as a core.TransferTarget
// so that liveResourceHandle.TransferOwn can dispatch into a non-table
// canon target (notably *canon.ValOwnHandle). IssueOwn translates the
// type, forwards to canon, and packages the canon handle back as a
// holderHandle for return through the core boundary.
type canonTargetShim struct{ ct canon.TransferTarget }

func (canonTargetShim) transferTarget() {}

func (s canonTargetShim) IssueOwn(tr *TypeResource, rep uint32) ResourceHandle {
	canonH := s.ct.IssueOwn(resourceType{rt: tr}, rep)
	return holderHandle{canonH: canonH}
}

// holderHandle wraps a canon.ResourceHandle as a core.ResourceHandle
// solely so canonTargetShim.IssueOwn can return through the core
// boundary. canonResourceHandleView.TransferOwn unwraps it. Methods
// other than HandleID and Rep panic — the holder never reaches user
// code through the canon API.
type holderHandle struct{ canonH canon.ResourceHandle }

func (h holderHandle) HandleID() uint32             { return h.canonH.HandleID() }
func (h holderHandle) Rep() uint32                  { return h.canonH.Rep() }
func (h holderHandle) Type() *TypeResource          { return nil }
func (h holderHandle) Instance() *ComponentInstance { return nil }
func (h holderHandle) LendTo(TransferTarget, *canon.Task) (ResourceHandle, error) {
	panic("LendTo on holderHandle")
}
func (h holderHandle) TransferOwn(TransferTarget) (ResourceHandle, error) {
	panic("TransferOwn on holderHandle")
}
func (h holderHandle) Drop(context.Context) error { panic("Drop on holderHandle") }

func (v canonResourceHandleView) Drop(ctx context.Context) error {
	return v.h.Drop(ctx)
}

// unwrapCanonTable extracts the underlying *ResourceTable from a
// canon.ResourceTable interface value. The only production wrapper is
// canonResourceTableView; returns nil for any other type.
func unwrapCanonTable(t canon.ResourceTable) *ResourceTable {
	if v, ok := t.(canonResourceTableView); ok {
		return v.t
	}
	return nil
}

var _ canon.ResourceHandle = canonResourceHandleView{}

// canonType adapts a parent-package Type to canon.Type. Accept drives a
// canon.TypeVisitor over the type tree via centralized dispatch, keeping
// public Type* types free of canon-specific methods.
type canonType struct{ t Type }

// Accept drives a canon.TypeVisitor for the wrapped type.
func (ct canonType) Accept(v canon.TypeVisitor) {
	switch t := ct.t.(type) {
	case TypeBool:
		v.VisitBool()
	case TypeU8:
		v.VisitU8()
	case TypeU16:
		v.VisitU16()
	case TypeU32:
		v.VisitU32()
	case TypeU64:
		v.VisitU64()
	case TypeS8:
		v.VisitS8()
	case TypeS16:
		v.VisitS16()
	case TypeS32:
		v.VisitS32()
	case TypeS64:
		v.VisitS64()
	case TypeF32:
		v.VisitF32()
	case TypeF64:
		v.VisitF64()
	case TypeChar:
		v.VisitChar()
	case TypeString:
		v.VisitString()
	case TypeFlags:
		v.VisitFlags(t.Names)
	case TypeEnum:
		v.VisitEnum(uint32(len(t.Cases)))
	case TypeOwn:
		v.VisitOwn(resourceType{rt: t.ResourceType})
	case TypeBorrow:
		v.VisitBorrow(resourceType{rt: t.ResourceType})
	case TypeList:
		v.VisitList(canonType{t: t.Elem})
	case TypeRecord:
		fields := make([]canon.RecordField, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = canon.RecordField{Name: f.Name, Type: canonType{t: f.Type}}
		}
		v.VisitRecord(fields)
	case TypeTuple:
		types := make([]canon.Type, len(t.Types))
		for i, ty := range t.Types {
			types[i] = canonType{t: ty}
		}
		v.VisitTuple(types)
	case TypeVariant:
		cases := make([]canon.VariantCase, len(t.Cases))
		for i, c := range t.Cases {
			cases[i].Name = c.Name
			if c.Payload != nil {
				cases[i].Payload = canonType{t: c.Payload}
			}
		}
		v.VisitVariant(cases)
	case TypeOption:
		v.VisitOption(canonType{t: t.Inner})
	case TypeResult:
		var okT, errT canon.Type
		if t.Ok != nil {
			okT = canonType{t: t.Ok}
		}
		if t.Err != nil {
			errT = canonType{t: t.Err}
		}
		v.VisitResult(okT, errT)
	default:
		panic(fmt.Sprintf("canon: unsupported wacogo Type %T", ct.t))
	}
}

// FuncTypeParamsAsCanon returns the params of ft as a []canon.Type slice
// suitable for compileTransferPlan / NewCallBinding.
func FuncTypeParamsAsCanon(ft *FuncType) []canon.Type {
	if ft == nil {
		return nil
	}
	out := make([]canon.Type, len(ft.Params))
	for i, p := range ft.Params {
		out[i] = canonType{t: p.Type}
	}
	return out
}

// FuncTypeResultsAsCanon is the results-slice counterpart.
func FuncTypeResultsAsCanon(ft *FuncType) []canon.Type {
	if ft == nil {
		return nil
	}
	out := make([]canon.Type, len(ft.Results))
	for i, r := range ft.Results {
		out[i] = canonType{t: r.Type}
	}
	return out
}
