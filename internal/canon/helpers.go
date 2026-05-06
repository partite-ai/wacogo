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
