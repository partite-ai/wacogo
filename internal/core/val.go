package core

import "github.com/partite-ai/wacogo/internal/canon"

// The canonical definitions of Val and its concrete variants live in
// internal/canon. This file re-exports them so user code can continue to
// write wacogo.ValU32(42), wacogo.NewValRecord(...), etc., without an
// extra layer of conversion between the parent and canon packages.

// Val is the interface implemented by all component model value types.
type Val = canon.Val

// --- Primitive types ---

// ValBool is a component model bool value.
type ValBool = canon.ValBool

// ValS8 is a component model s8 value.
type ValS8 = canon.ValS8

// ValU8 is a component model u8 value.
type ValU8 = canon.ValU8

// ValS16 is a component model s16 value.
type ValS16 = canon.ValS16

// ValU16 is a component model u16 value.
type ValU16 = canon.ValU16

// ValS32 is a component model s32 value.
type ValS32 = canon.ValS32

// ValU32 is a component model u32 value.
type ValU32 = canon.ValU32

// ValS64 is a component model s64 value.
type ValS64 = canon.ValS64

// ValU64 is a component model u64 value.
type ValU64 = canon.ValU64

// ValF32 is a component model f32 value.
type ValF32 = canon.ValF32

// ValF64 is a component model f64 value.
type ValF64 = canon.ValF64

// ValChar is a component model char value.
type ValChar = canon.ValChar

// ValString is a component model string value.
type ValString = canon.ValString

// --- Compound types ---

// ValList holds an ordered sequence of Val elements.
type ValList = canon.ValList

// Field is a named field in a record value.
type Field = canon.Field

// ValRecord holds an ordered set of named fields.
type ValRecord = canon.ValRecord

// ValVariant represents a variant case identified by a discriminant and optional payload.
type ValVariant = canon.ValVariant

// ValEnum represents an enum case identified by a discriminant.
type ValEnum = canon.ValEnum

// ValOption represents an optional value (some or none).
type ValOption = canon.ValOption

// ValResult represents either an ok value or an error value.
type ValResult = canon.ValResult

// ValFlags represents a set of named boolean flags packed into uint32 words.
type ValFlags = canon.ValFlags

// ValOwnHandle is an owned resource handle.
type ValOwnHandle = canon.ValOwnHandle

// NewValOwnHandle constructs a host-created owned resource value suitable
// for passing to Func.Call. rep is interpreted by rt's defining instance.
func NewValOwnHandle(rt *TypeResource, rep uint32) *ValOwnHandle {
	if rt == nil {
		panic("wacogo: NewValOwnHandle: nil resource type")
	}
	return canon.NewValOwnHandle(resourceType{rt: rt}, rep)
}

// --- Non-generic constructors (re-exported as variable aliases) ---

// NewValRecord constructs a record from the given fields in order.
var NewValRecord = canon.NewValRecord

// NewValVariant constructs a variant value. payload may be nil.
var NewValVariant = canon.NewValVariant

// NewValEnum constructs an enum value.
var NewValEnum = canon.NewValEnum

// ValOptionNone constructs a none option.
var ValOptionNone = canon.ValOptionNone

// ValOptionSome constructs a some option wrapping v.
var ValOptionSome = canon.ValOptionSome

// ValResultOk constructs an ok result.
var ValResultOk = canon.ValResultOk

// ValResultErr constructs an error result.
var ValResultErr = canon.ValResultErr

// NewValFlags constructs a flags value with the given flag names, with the
// specified flags pre-set. Unknown names in set are silently ignored.
var NewValFlags = canon.NewValFlags

// --- Generic constructors (thin wrappers — Go doesn't support type-aliased
// generic function values) ---

// NewValListOf constructs a ValList from elements of a concrete Val type.
func NewValListOf[T Val](elems ...T) *ValList {
	return canon.NewValListOf[T](elems...)
}
