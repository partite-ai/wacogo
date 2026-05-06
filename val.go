package wacogo

import "github.com/partite-ai/wacogo/internal/core"

// Val is the interface satisfied by every component-model runtime value.
type Val = core.Val

// ValBool is a component-model bool value.
type ValBool = core.ValBool

// ValU8 is a component-model u8 value.
type ValU8 = core.ValU8

// ValU16 is a component-model u16 value.
type ValU16 = core.ValU16

// ValU32 is a component-model u32 value.
type ValU32 = core.ValU32

// ValU64 is a component-model u64 value.
type ValU64 = core.ValU64

// ValS8 is a component-model s8 value.
type ValS8 = core.ValS8

// ValS16 is a component-model s16 value.
type ValS16 = core.ValS16

// ValS32 is a component-model s32 value.
type ValS32 = core.ValS32

// ValS64 is a component-model s64 value.
type ValS64 = core.ValS64

// ValF32 is a component-model f32 value.
type ValF32 = core.ValF32

// ValF64 is a component-model f64 value.
type ValF64 = core.ValF64

// ValChar is a component-model char value.
type ValChar = core.ValChar

// ValString is a component-model string value.
type ValString = core.ValString

// ValList holds an ordered sequence of Val elements.
type ValList = core.ValList

// ValOption represents an optional value (some or none).
type ValOption = core.ValOption

// ValResult represents either an ok value or an error value.
type ValResult = core.ValResult

// ValRecord holds an ordered set of named fields.
type ValRecord = core.ValRecord

// ValVariant represents a variant case identified by a discriminant and
// an optional payload.
type ValVariant = core.ValVariant

// ValFlags represents a set of named boolean flags.
type ValFlags = core.ValFlags

// ValEnum represents an enum case identified by a discriminant.
type ValEnum = core.ValEnum

// ValOwnHandle is an owned resource handle value.
type ValOwnHandle = core.ValOwnHandle

// NewValListOf constructs a *ValList whose elements are elems.
func NewValListOf[T Val](elems ...T) *ValList {
	return core.NewValListOf[T](elems...)
}

// NewValRecord constructs a record from the given fields in order.
func NewValRecord(fields ...Field) *ValRecord {
	return core.NewValRecord(fields...)
}

// NewValVariant constructs a variant value with the given discriminant
// and payload. payload may be nil for cases that carry no value.
func NewValVariant(discriminant uint32, payload Val) *ValVariant {
	return core.NewValVariant(discriminant, payload)
}

// NewValFlags constructs a flags value with the given flag names and
// the specified flags pre-set. Unknown names in set are silently
// ignored.
func NewValFlags(names []string, set ...string) *ValFlags {
	return core.NewValFlags(names, set...)
}

// NewValEnum constructs an enum value with the given case discriminant.
func NewValEnum(discriminant uint32) *ValEnum {
	return core.NewValEnum(discriminant)
}

// ValOptionSome constructs an option carrying v.
func ValOptionSome(v Val) *ValOption {
	return core.ValOptionSome(v)
}

// ValOptionNone constructs a none option.
func ValOptionNone() *ValOption {
	return core.ValOptionNone()
}

// ValResultOk constructs an ok result carrying v.
func ValResultOk(v Val) *ValResult {
	return core.ValResultOk(v)
}

// ValResultErr constructs an error result carrying v.
func ValResultErr(v Val) *ValResult {
	return core.ValResultErr(v)
}
