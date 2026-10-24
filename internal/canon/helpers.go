package canon

import (
	"errors"
	"fmt"
)

// Trap is a recoverable panic value carrying a trap message. Call-path
// entry points recover and convert to either a Go error (Func.Call) or
// a wazero trap (adapterFunc.Call). err, when set, is the underlying
// error the trap reports, kept so callers can match it with errors.Is.
type Trap struct {
	msg string
	err error
}

func (t *Trap) Error() string { return t.msg }

func (t *Trap) Unwrap() error { return t.err }

// asError converts a recovered trap into the error returned across a
// Go-facing call boundary, without exposing *Trap itself.
func (t *Trap) asError() error {
	if t.err != nil {
		return t.err
	}
	return errors.New(t.msg)
}

func fmtSprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// poisonOnPanic poisons inst if a panic is in flight, then re-raises it.
// Must be invoked directly via defer. Used around the window between the
// callee returning and its post-return completing: a failure there means
// the post-return the callee is relying on will never run, so the callee
// is treated as though it trapped.
func poisonOnPanic(inst Instance) {
	if r := recover(); r != nil {
		if err, ok := r.(error); ok {
			inst.Poison(err)
		} else {
			inst.Poison(fmt.Errorf("%v", r))
		}
		panic(r)
	}
}
