package wasmparser

import "fmt"

// Error represents a parsing or validation error at a specific byte offset.
type Error struct {
	Offset  uint64
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("at offset %d: %s", e.Offset, e.Message)
}

func errAt(offset uint64, msg string) *Error {
	return &Error{Offset: offset, Message: msg}
}

func errfAt(offset uint64, format string, args ...any) *Error {
	return &Error{Offset: offset, Message: fmt.Sprintf(format, args...)}
}
