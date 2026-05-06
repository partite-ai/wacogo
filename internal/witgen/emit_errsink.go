package witgen

import "fmt"

// errSink emits the failure-return statement for a helper. The shape
// varies: a lift helper returns (T, error) so its sink emits
// `return <zeroValue>, <errExpr>`; a lower helper returns just error
// so its sink emits `return <errExpr>`.
//
// Concrete sites pass in errExpr as either a bare ident ("err") or a
// fmt.Errorf expression. The sink wraps it in the correct return form.
type errSink interface {
	Emit(errExpr string) string
}

// errSinkLower is the failure-return for helpers that return error
// alone (lowerFlat<T>, lowerMem<T>, fromGoFlat<T>, fromGoMem<T>).
type errSinkLower struct{}

func (errSinkLower) Emit(errExpr string) string {
	return "return " + errExpr
}

// errSinkLift is the failure-return for helpers that return (T, error)
// (liftFlat<T>, liftMem<T>, toGoFlat<T>, toGoMem<T>). ZeroValueExpr
// is the Go expression producing a zero-valued T (e.g. "0" for uint32,
// `""` for string, "MyStruct{}" for a struct, "nil" for a slice or
// pointer).
type errSinkLift struct {
	ZeroValueExpr string
}

func (s errSinkLift) Emit(errExpr string) string {
	return fmt.Sprintf("return %s, %s", s.ZeroValueExpr, errExpr)
}
