package wasmparser

import (
	"errors"
	"testing"
)

func TestError(t *testing.T) {
	err := &Error{Offset: 42, Message: "unexpected byte"}
	if err.Error() != "at offset 42: unexpected byte" {
		t.Fatalf("unexpected error string: %s", err.Error())
	}

	var e error = err
	var pe *Error
	if !errors.As(e, &pe) {
		t.Fatal("Error should implement error interface")
	}
	if pe.Offset != 42 {
		t.Fatalf("expected offset 42, got %d", pe.Offset)
	}
}
