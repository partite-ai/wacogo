package witgen

import (
	"testing"
)

func TestErrSinkLowerForm(t *testing.T) {
	// Lower-helpers return just error.
	s := errSinkLower{}
	got := s.Emit("err")
	want := "return err"
	if got != want {
		t.Fatalf("errSinkLower.Emit: got %q, want %q", got, want)
	}
}

func TestErrSinkLiftForm(t *testing.T) {
	// Lift-helpers return (zeroExpr, error).
	s := errSinkLift{ZeroValueExpr: "MyType{}"}
	got := s.Emit("err")
	want := "return MyType{}, err"
	if got != want {
		t.Fatalf("errSinkLift.Emit: got %q, want %q", got, want)
	}
}
