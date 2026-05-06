package outgoinghandler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/partite-ai/wacogo/wasi/http/types"
)

func TestTranslateResultErr_CodedErrorHTTPRequestDenied(t *testing.T) {
	in := &types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}
	got := translateResultErr(in)
	if _, ok := got.(types.ErrorCodeHTTPRequestDenied); !ok {
		t.Fatalf("got %T, want ErrorCodeHTTPRequestDenied", got)
	}
}

func TestTranslateResultErr_CodedErrorDestinationIPProhibited(t *testing.T) {
	in := &types.CodedError{Code: types.ErrorCodeDestinationIPProhibited{}}
	got := translateResultErr(in)
	if _, ok := got.(types.ErrorCodeDestinationIPProhibited); !ok {
		t.Fatalf("got %T, want ErrorCodeDestinationIPProhibited", got)
	}
}

func TestTranslateResultErr_CodedErrorWrapped(t *testing.T) {
	inner := &types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}
	wrapped := fmt.Errorf("doer rejected: %w", inner)
	got := translateResultErr(wrapped)
	if _, ok := got.(types.ErrorCodeHTTPRequestDenied); !ok {
		t.Fatalf("got %T, want ErrorCodeHTTPRequestDenied (errors.As must unwrap)", got)
	}
}

func TestTranslateResultErr_CodedErrorNilCodeFallsThrough(t *testing.T) {
	in := &types.CodedError{Code: nil, Msg: "unspecific deny"}
	got := translateResultErr(in)
	internal, ok := got.(types.ErrorCodeInternalError)
	if !ok {
		t.Fatalf("got %T, want ErrorCodeInternalError fallback", got)
	}
	if !internal.Value.IsSome || internal.Value.Value != "unspecific deny" {
		t.Fatalf("ErrorCodeInternalError.Value = %+v, want SomeString(\"unspecific deny\")", internal.Value)
	}
}

func TestTranslateResultErr_PlainErrorStillFallsBack(t *testing.T) {
	got := translateResultErr(errors.New("blocked"))
	internal, ok := got.(types.ErrorCodeInternalError)
	if !ok {
		t.Fatalf("got %T, want ErrorCodeInternalError", got)
	}
	if !internal.Value.IsSome || internal.Value.Value != "blocked" {
		t.Fatalf("ErrorCodeInternalError.Value = %+v, want SomeString(\"blocked\")", internal.Value)
	}
}
