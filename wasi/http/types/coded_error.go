package types

import "fmt"

// CodedError carries a wasi:http/types ErrorCode as a Go error.
//
// Returning a *CodedError from an HTTPDoer surfaces Code to the guest
// instead of the default ErrorCodeInternalError mapping. If Msg is
// empty, Error() falls back to a description naming the Code's Go type.
type CodedError struct {
	Code ErrorCode
	Msg  string
}

func (e *CodedError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("wasi:http error %T", e.Code)
}
