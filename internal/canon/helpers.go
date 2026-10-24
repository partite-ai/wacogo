package canon

import (
	"fmt"
)

// Trap is a recoverable panic value carrying a trap message. Call-path
// entry points recover and convert to either a Go error (Func.Call) or
// a wazero trap (adapterFunc.Call).
type Trap struct{ msg string }

func (t *Trap) Error() string { return t.msg }

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
