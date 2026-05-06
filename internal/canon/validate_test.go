package canon

import (
	"errors"
	"testing"
)

func TestValidateCharRejectsOutOfRange(t *testing.T) {
	if err := validateChar(0x110000); err == nil {
		t.Fatal("expected error for out-of-range code point")
	}
}

func TestValidateCharRejectsSurrogates(t *testing.T) {
	if err := validateChar(0xD800); err == nil {
		t.Fatal("expected error for surrogate")
	}
	if err := validateChar(0xDFFF); err == nil {
		t.Fatal("expected error for surrogate")
	}
}

func TestValidateCharAccepts(t *testing.T) {
	for _, cp := range []rune{'A', 0x80, 0xFFFF, 0x10FFFF} {
		if err := validateChar(cp); err != nil {
			t.Fatalf("rejected valid char %#x: %v", cp, err)
		}
	}
}

func TestCheckListLenOverflow(t *testing.T) {
	err := checkListLen(1<<30, 8)
	if err == nil || !errors.Is(err, errListByteLength) {
		t.Fatalf("expected errListByteLength, got %v", err)
	}
}

func TestCheckListLenZero(t *testing.T) {
	if err := checkListLen(0, 1); err != nil {
		t.Fatalf("zero-length list rejected: %v", err)
	}
}

func TestValidateFlagsLabelCount(t *testing.T) {
	if err := validateFlagsLabelCount(32); err != nil {
		t.Fatalf("32 labels rejected: %v", err)
	}
	if err := validateFlagsLabelCount(33); err == nil {
		t.Fatal("expected error for >32 labels")
	}
}

func TestCheckAlignment(t *testing.T) {
	if err := checkAlignment(16, 8); err != nil {
		t.Fatalf("aligned ptr rejected: %v", err)
	}
	if err := checkAlignment(17, 8); err == nil {
		t.Fatal("expected error for misaligned ptr")
	}
}
