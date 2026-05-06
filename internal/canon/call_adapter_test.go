package canon

import (
	"context"
	"testing"
)

func TestCallAdapter_CloseIdempotent(t *testing.T) {
	a := &CallAdapter{}
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
	// Nil receiver safe.
	var nilA *CallAdapter
	if err := nilA.Close(context.Background()); err != nil {
		t.Fatalf("nil close: %v", err)
	}
}
