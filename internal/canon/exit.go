package canon

import (
	"context"
	"errors"
	"fmt"
)

// exitWithError is deferred by error-returning call boundaries. Exit has no
// error result, but host cleanup can panic; retain both the call's error and
// cleanup failure. A panic already unwinding the call remains a panic.
func exitWithError(ctx context.Context, instance Instance, result *error) {
	original := recover()
	cleanupErr := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				if cause, ok := p.(error); ok {
					err = cause
				} else {
					err = fmt.Errorf("component exit: %v", p)
				}
			}
		}()
		instance.Exit(ctx)
		return nil
	}()
	if original != nil {
		if cleanupErr == nil {
			panic(original)
		}
		if cause, ok := original.(error); ok {
			panic(errors.Join(cause, *result, cleanupErr))
		}
		panic(errors.Join(fmt.Errorf("component call panic: %v", original), *result, cleanupErr))
	}
	*result = errors.Join(*result, cleanupErr)
}
