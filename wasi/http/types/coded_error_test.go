package types_test

import (
	"testing"

	"github.com/partite-ai/wacogo/wasi/http/types"
)

func TestCodedError_Error_UsesMsgWhenSet(t *testing.T) {
	err := &types.CodedError{
		Code: types.ErrorCodeHTTPRequestDenied{},
		Msg:  "blocked by policy",
	}
	if got, want := err.Error(), "blocked by policy"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCodedError_Error_DefaultIncludesCodeType(t *testing.T) {
	err := &types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}
	got := err.Error()
	want := "wasi:http error types.ErrorCodeHTTPRequestDenied"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCodedError_Error_NilCode(t *testing.T) {
	err := &types.CodedError{}
	got := err.Error()
	want := "wasi:http error <nil>"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
