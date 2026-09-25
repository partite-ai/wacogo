package canon

import (
	"context"
	"fmt"
	"os"
	"runtime"
)

// Val is the interface implemented by all component model value types.
// The private marker method prevents external implementations.
//
// This file owns the canonical Val type definitions; the parent wacogo
// package re-exports each type via a Go type alias. Keeping the concrete
// definitions in internal/canon removes a layer of Val↔Val conversion
// around the canon step pipeline.
type Val interface {
	val()
}

// --- Primitive types ---

// ValBool is a component model bool value.
type ValBool bool

func (ValBool) val() {}

// ValS8 is a component model s8 value.
type ValS8 int8

func (ValS8) val() {}

// ValU8 is a component model u8 value.
type ValU8 uint8

func (ValU8) val() {}

// ValS16 is a component model s16 value.
type ValS16 int16

func (ValS16) val() {}

// ValU16 is a component model u16 value.
type ValU16 uint16

func (ValU16) val() {}

// ValS32 is a component model s32 value.
type ValS32 int32

func (ValS32) val() {}

// ValU32 is a component model u32 value.
type ValU32 uint32

func (ValU32) val() {}

// ValS64 is a component model s64 value.
type ValS64 int64

func (ValS64) val() {}

// ValU64 is a component model u64 value.
type ValU64 uint64

func (ValU64) val() {}

// ValF32 is a component model f32 value.
type ValF32 float32

func (ValF32) val() {}

// ValF64 is a component model f64 value.
type ValF64 float64

func (ValF64) val() {}

// ValChar is a component model char value.
type ValChar rune

func (ValChar) val() {}

// ValString is a component model string value.
type ValString string

func (ValString) val() {}

// --- ValList ---

// ValList holds an ordered sequence of Val elements.
type ValList struct {
	elems []Val
}

// NewValListOf constructs a ValList from elements of a concrete Val type.
func NewValListOf[T Val](elems ...T) *ValList {
	out := make([]Val, len(elems))
	for i, e := range elems {
		out[i] = e
	}
	return &ValList{elems: out}
}

// newValListFromSlice constructs a ValList from a heterogeneous slice of
// Vals. Used by lift paths that materialize list elements one at a time
// (the public NewValListOf is generic and requires a homogeneous type).
// Internal to canon — external callers use NewValListOf.
func newValListFromSlice(elems []Val) *ValList {
	return &ValList{elems: elems}
}

func (l *ValList) val() {}

// Len returns the number of elements in the list.
func (l *ValList) Len() int { return len(l.elems) }

// Get returns the element at position i.
func (l *ValList) Get(i int) Val { return l.elems[i] }

// Append appends v to the list. Used by lift paths that accumulate
// elements as they are produced.
func (l *ValList) Append(v Val) { l.elems = append(l.elems, v) }

// --- ValRecord ---

// Field is a named field in a record value.
type Field struct {
	Name string
	Val  Val
}

// ValRecord holds an ordered set of named fields.
type ValRecord struct {
	fields []Field
	index  map[string]int // lazy
}

// NewValRecord constructs a record from the given fields in order.
func NewValRecord(fields ...Field) *ValRecord {
	cp := make([]Field, len(fields))
	copy(cp, fields)
	return &ValRecord{fields: cp}
}

func (r *ValRecord) val() {}

func (r *ValRecord) buildIndex() {
	if r.index != nil {
		return
	}
	r.index = make(map[string]int, len(r.fields))
	for i, f := range r.fields {
		r.index[f.Name] = i
	}
}

// Field returns the value of the named field, or nil if not found.
func (r *ValRecord) Field(name string) Val {
	r.buildIndex()
	i, ok := r.index[name]
	if !ok {
		return nil
	}
	return r.fields[i].Val
}

// FieldByIndex returns the value of the field at position i.
func (r *ValRecord) FieldByIndex(i int) Val { return r.fields[i].Val }

// Fields returns the ordered slice of fields.
func (r *ValRecord) Fields() []Field { return r.fields }

// --- ValVariant ---

// ValVariant represents a variant case identified by a discriminant and optional payload.
type ValVariant struct {
	discriminant uint32
	payload      Val
}

// NewValVariant constructs a variant value. payload may be nil.
func NewValVariant(discriminant uint32, payload Val) *ValVariant {
	return &ValVariant{discriminant: discriminant, payload: payload}
}

func (v *ValVariant) val() {}

// Discriminant returns the variant case index.
func (v *ValVariant) Discriminant() uint32 { return v.discriminant }

// Val returns the payload value, which may be nil.
func (v *ValVariant) Val() Val { return v.payload }

// --- ValEnum ---

// ValEnum represents an enum case identified by a discriminant.
type ValEnum struct {
	discriminant uint32
}

// NewValEnum constructs an enum value.
func NewValEnum(discriminant uint32) *ValEnum {
	return &ValEnum{discriminant: discriminant}
}

func (e *ValEnum) val() {}

// Discriminant returns the enum case index.
func (e *ValEnum) Discriminant() uint32 { return e.discriminant }

// --- ValOption ---

// ValOption represents an optional value (some or none).
type ValOption struct {
	payload Val // nil means none
}

// ValOptionNone constructs a none option.
func ValOptionNone() *ValOption { return &ValOption{} }

// ValOptionSome constructs a some option wrapping v.
func ValOptionSome(v Val) *ValOption { return &ValOption{payload: v} }

func (o *ValOption) val() {}

// IsNone reports whether the option is none.
func (o *ValOption) IsNone() bool { return o.payload == nil }

// Val returns the contained value. It panics if the option is none.
func (o *ValOption) Val() Val {
	if o.payload == nil {
		panic("Val called on none ValOption")
	}
	return o.payload
}

// --- ValResult ---

// ValResult represents either an ok value or an error value.
type ValResult struct {
	ok      bool
	payload Val
}

// ValResultOk constructs an ok result.
func ValResultOk(v Val) *ValResult { return &ValResult{ok: true, payload: v} }

// ValResultErr constructs an error result.
func ValResultErr(v Val) *ValResult { return &ValResult{ok: false, payload: v} }

func (r *ValResult) val() {}

// IsOk reports whether the result is ok.
func (r *ValResult) IsOk() bool { return r.ok }

// Ok returns the ok payload. It panics if the result is an error.
func (r *ValResult) Ok() Val {
	if !r.ok {
		panic("Ok called on error ValResult")
	}
	return r.payload
}

// Err returns the error payload. It panics if the result is ok.
func (r *ValResult) Err() Val {
	if r.ok {
		panic("Err called on ok ValResult")
	}
	return r.payload
}

// --- ValFlags ---

// ValFlags represents a set of named boolean flags packed into uint32 words.
type ValFlags struct {
	names []string
	index map[string]int // name -> bit position
	bits  []uint32
}

// NewValFlags constructs a flags value with the given flag names, with the specified flags
// pre-set. Unknown names in set are silently ignored.
func NewValFlags(names []string, set ...string) *ValFlags {
	nwords := (len(names) + 31) / 32
	idx := make(map[string]int, len(names))
	for i, n := range names {
		idx[n] = i
	}
	f := &ValFlags{
		names: names,
		index: idx,
		bits:  make([]uint32, nwords),
	}
	for _, name := range set {
		f.Set(name)
	}
	return f
}

// newValFlagsFromBits constructs a ValFlags from a raw packed u32 with no
// name labels. Retained as a test-only helper for lower/transfer visitors
// that need to inject out-of-range bits and observe masking.
func newValFlagsFromBits(bits uint32) *ValFlags {
	return &ValFlags{bits: []uint32{bits}}
}

// buildFlagsIndex builds the name → bit-position map used by ValFlags.
// Called once per VisitFlags dispatch at plan-compile time; the resulting
// map is shared across every lifted *ValFlags produced by that plan, so
// the per-call cost is a single struct allocation.
func buildFlagsIndex(names []string) map[string]int {
	idx := make(map[string]int, len(names))
	for i, n := range names {
		idx[n] = i
	}
	return idx
}

// newLiftedValFlags constructs a *ValFlags for a freshly lifted value.
// names and index are captured at plan-compile time and shared across
// every call; only bits is per-call.
func newLiftedValFlags(names []string, index map[string]int, bits uint32) *ValFlags {
	return &ValFlags{names: names, index: index, bits: []uint32{bits}}
}

func (f *ValFlags) val() {}

// Has reports whether the named flag is set.
func (f *ValFlags) Has(name string) bool {
	bit, ok := f.index[name]
	if !ok {
		return false
	}
	word, mask := bit/32, uint32(1)<<(uint(bit)%32)
	return f.bits[word]&mask != 0
}

// Set sets the named flag. Unknown names are silently ignored.
func (f *ValFlags) Set(name string) {
	bit, ok := f.index[name]
	if !ok {
		return
	}
	word, mask := bit/32, uint32(1)<<(uint(bit)%32)
	f.bits[word] |= mask
}

// Clear clears the named flag. Unknown names are silently ignored.
func (f *ValFlags) Clear(name string) {
	bit, ok := f.index[name]
	if !ok {
		return
	}
	word, mask := bit/32, uint32(1)<<(uint(bit)%32)
	f.bits[word] &^= mask
}

// Bits returns the packed bit words representing the flag set.
func (f *ValFlags) Bits() []uint32 { return f.bits }

// packedBits returns the first u32 of the packed bit representation, which
// is the only word in use at the MVP (canon caps flag labels at 32).
// Internal to canon — external callers should use Bits() for the full
// packed representation.
func (f *ValFlags) packedBits() uint32 {
	if len(f.bits) == 0 {
		return 0
	}
	return f.bits[0]
}

// --- ValOwnHandle ---

// valHandleState tracks a *ValOwnHandle's lifecycle:
//
//	empty       — fresh, awaiting IssueOwn (zero value).
//	valid       — populated, holds (rt, rep), can lend / transfer / drop.
//	transferred — TransferOwn'd out; rt and rep are no longer meaningful.
//	dropped     — Drop'd; dtor has run.
type valHandleState uint8

const (
	valHandleEmpty valHandleState = iota
	valHandleValid
	valHandleTransferred
	valHandleDropped
)

// ValOwnHandle is a value carrying an owned resource handle. It is
// itself a TransferTarget *and* a ResourceHandle: a source's
// TransferOwn(val) populates the Val with (rt, rep), and the Val can
// subsequently lower as own (TransferOwn into a callee table) or
// borrow (LendTo) depending on what the call expects.
//
// The gocall path mints ValOwnHandle values when lifting them out of a
// component call. Hosts may also create one explicitly with
// NewValOwnHandle when they already own a resource representation that
// must be transferred through Func.Call.
type ValOwnHandle struct {
	rt       ResourceType
	rep      uint32
	numLends uint32
	state    valHandleState
	cleanup  runtime.Cleanup
}

func (h *ValOwnHandle) val() {}

// IssueOwn implements TransferTarget. The source's TransferOwn invokes
// this once, handing the Val (rt, rep). Registers the leak-warning
// cleanup at this point because (rt, rep) are not known until now.
// Returns h itself so the source's TransferOwn has a ResourceHandle
// to return.
func (h *ValOwnHandle) IssueOwn(rt ResourceType, rep uint32) ResourceHandle {
	h.rt = rt
	h.rep = rep
	h.state = valHandleValid
	h.cleanup = runtime.AddCleanup(h, leakWarn, leakInfo{rt: rt, rep: rep})
	return h
}

func (h *ValOwnHandle) HandleID() uint32 { return h.rep }
func (h *ValOwnHandle) Rep() uint32 {
	if h.state != valHandleValid {
		panic(fmt.Sprintf("Rep on %s ValOwnHandle", h.stateName()))
	}
	return h.rep
}
func (h *ValOwnHandle) Type() ResourceType {
	if h.state == valHandleDropped {
		return nil
	}
	return h.rt
}

func (h *ValOwnHandle) LendTo(target ResourceTable, task *Task) (ResourceHandle, error) {
	if h.state != valHandleValid {
		return nil, fmt.Errorf("LendTo on %s ValOwnHandle", h.stateName())
	}
	if target == nil {
		return nil, fmt.Errorf("LendTo: nil target")
	}
	h.numLends++
	src := h
	task.AddRelease(func() {
		if src.numLends > 0 {
			src.numLends--
		}
	})

	// Same-component shortcut: target's owner equals the resource's
	// defining instance. Rep flows through unchanged with no callee
	// allocation, no NumBorrows increment. The returned carrier exists
	// solely so the visitor can write HandleID() == rep into the wasm
	// slot.
	if h.rt != nil && h.rt.DefiningInstance() != nil && target.Owner() == h.rt.DefiningInstance() {
		return &repCarrier{rep: h.rep}, nil
	}
	return target.IssueBorrow(h.rt, h.rep, task), nil
}

func (h *ValOwnHandle) TransferOwn(target TransferTarget) (ResourceHandle, error) {
	if h.state != valHandleValid {
		return nil, fmt.Errorf("TransferOwn on %s ValOwnHandle", h.stateName())
	}
	if h.numLends > 0 {
		return nil, fmt.Errorf("cannot transfer ValOwnHandle with %d outstanding borrows", h.numLends)
	}
	if target == nil {
		return nil, fmt.Errorf("TransferOwn: nil target")
	}
	rt := h.rt
	rep := h.rep
	h.state = valHandleTransferred
	h.stopCleanup()
	return target.IssueOwn(rt, rep), nil
}

// Drop releases the resource. Runs the destructor (with defining-
// instance Enter gating). Idempotent on already-dropped handles and
// no-op on empty (never-populated) handles; errors on handles that
// were transferred out by a prior call.
func (h *ValOwnHandle) Drop(ctx context.Context) error {
	if h == nil {
		return nil
	}
	switch h.state {
	case valHandleEmpty, valHandleDropped:
		return nil
	case valHandleTransferred:
		return fmt.Errorf("Drop on already transferred ValOwnHandle")
	}
	if h.numLends > 0 {
		return fmt.Errorf("cannot drop ValOwnHandle with %d outstanding borrows", h.numLends)
	}
	rt := h.rt
	rep := h.rep
	h.state = valHandleDropped
	h.stopCleanup()
	dtor := rt.Destructor()
	if dtor == nil {
		return nil
	}
	defining := rt.DefiningInstance()
	if defining == nil {
		return dtor(ctx, rep)
	}
	if err := defining.Enter(ctx); err != nil {
		return err
	}
	defer defining.Exit(ctx)
	return dtor(ctx, rep)
}

func (h *ValOwnHandle) stopCleanup() {
	cleanup := h.cleanup
	h.cleanup = runtime.Cleanup{}
	cleanup.Stop()
	// Keep h reachable across Stop so the runtime cannot queue the cleanup
	// concurrently just before Stop removes it.
	runtime.KeepAlive(h)
}

func (h *ValOwnHandle) stateName() string {
	switch h.state {
	case valHandleEmpty:
		return "empty"
	case valHandleTransferred:
		return "already transferred"
	case valHandleDropped:
		return "already dropped"
	default:
		return "valid"
	}
}

// newValOwnHandleEmpty mints an empty Val ready to receive ownership
// via IssueOwn. The leak-warning cleanup is registered when IssueOwn
// fires.
func newValOwnHandleEmpty() *ValOwnHandle {
	return &ValOwnHandle{}
}

// NewValOwnHandle constructs an owned resource value from a resource type
// and a representation owned by that type's defining instance.
func NewValOwnHandle(rt ResourceType, rep uint32) *ValOwnHandle {
	h := newValOwnHandleEmpty()
	h.IssueOwn(rt, rep)
	return h
}

// NewValOwnHandleForTest mints a *ValOwnHandle pre-populated with
// (rt, rep). Test-only; exported so cross-package tests (notably
// internal/core integration tests) can synthesize Vals when
// exercising the gocall path. No leak cleanup is registered.
func NewValOwnHandleForTest(rt ResourceType, rep uint32) *ValOwnHandle {
	return &ValOwnHandle{rt: rt, rep: rep, state: valHandleValid}
}

type leakInfo struct {
	rt  ResourceType
	rep uint32
}

func leakWarn(info leakInfo) {
	rtName := "<nil>"
	if info.rt != nil {
		rtName = fmt.Sprintf("%T", info.rt)
	}
	fmt.Fprintf(os.Stderr,
		"wacogo: warning: *ValOwnHandle leaked (type=%s rep=%d) — Drop was never called\n",
		rtName, info.rep)
}

// repCarrier is the canon-side analog of core's stripped
// detachedResourceHandle: a transient ResourceHandle whose only
// meaningful operation is HandleID(), used as the same-component
// LendTo shortcut return so the visitor can write rep into the wasm
// slot. Other methods panic — the carrier is never user-facing and
// never lives past the visitor's HandleID() read.
type repCarrier struct{ rep uint32 }

func (c *repCarrier) HandleID() uint32   { return c.rep }
func (c *repCarrier) Rep() uint32        { return c.rep }
func (c *repCarrier) Type() ResourceType { return nil }
func (c *repCarrier) LendTo(ResourceTable, *Task) (ResourceHandle, error) {
	panic("LendTo on repCarrier")
}
func (c *repCarrier) TransferOwn(TransferTarget) (ResourceHandle, error) {
	panic("TransferOwn on repCarrier")
}
func (c *repCarrier) Drop(context.Context) error {
	panic("Drop on repCarrier")
}
