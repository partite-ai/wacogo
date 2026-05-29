package canon

import (
	"errors"
	"fmt"
)

var (
	errListByteLength = errors.New("list byte length exceeds max or overflows")
)

// validateChar checks that r is a valid Unicode scalar value. The error
// messages carry the substring "invalid `char` bit pattern" so spec tests
// matching on that phrase pass regardless of which pipeline (gocall or
// transfer) ran the validation.
func validateChar(r rune) error {
	if r >= 0x110000 {
		return fmt.Errorf("invalid `char` bit pattern: code point %#x out of range", r)
	}
	if r >= 0xD800 && r <= 0xDFFF {
		return fmt.Errorf("invalid `char` bit pattern: surrogate code point %#x disallowed", r)
	}
	return nil
}

// checkListLen returns an error if length*elemSize overflows or exceeds
// MaxListByteLength. length=0 always succeeds. elemSize=0 (empty record /
// tuple element types) is treated as having zero in-memory footprint
// regardless of length — the canonical ABI does not require allocation
// for such lists.
func checkListLen(length, elemSize uint32) error {
	if length == 0 || elemSize == 0 {
		return nil
	}
	byteLen := length * elemSize
	if byteLen/elemSize != length || byteLen > MaxListByteLength {
		return errListByteLength
	}
	return nil
}

// validateFlagsLabelCount enforces the MVP cap (32 labels).
func validateFlagsLabelCount(n uint32) error {
	if n > 32 {
		return fmt.Errorf("flags: label count %d exceeds cap 32", n)
	}
	return nil
}

// flagsByteSize returns the in-memory byte size and alignment of a flags
// value with n labels per the canonical ABI (size == alignment): 1 for
// n<=8, 2 for n<=16, 4 for n<=32.
func flagsByteSize(n uint32) uint32 {
	switch {
	case n <= 8:
		return 1
	case n <= 16:
		return 2
	default:
		return 4
	}
}

// checkAlignment returns an error if ptr is not aligned to align.
// align==0 and align==1 always succeed.
func checkAlignment(ptr, align uint32) error {
	if align <= 1 {
		return nil
	}
	if ptr%align != 0 {
		return fmt.Errorf("unaligned pointer %d (align %d)", ptr, align)
	}
	return nil
}

// mustTransfer wraps an error-returning helper call with panic(&Trap{}) for
// transfer-step call sites. Unused-in-gocall: gocall steps return directly.
func mustTransfer(err error) {
	if err != nil {
		panic(&Trap{msg: err.Error()})
	}
}
