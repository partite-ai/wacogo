package wasmparser

import (
	"reflect"
	"testing"
)

// TestParseLimits_Resolved checks that every field has a default, so a limit
// left unset can't mean rejecting everything, and that every field can be
// overridden.
func TestParseLimits_Resolved(t *testing.T) {
	const override = 12345

	var set ParseLimits
	rv := reflect.ValueOf(&set).Elem()
	for i := range rv.NumField() {
		rv.Field(i).SetUint(override)
	}
	resolved := reflect.ValueOf(set.resolved())
	defaults := reflect.ValueOf(ParseLimits{}.resolved())
	for i := range rv.NumField() {
		name := rv.Type().Field(i).Name
		if got := resolved.Field(i).Uint(); got != override {
			t.Errorf("%s: override gave %d", name, got)
		}
		if defaults.Field(i).Uint() == 0 {
			t.Errorf("%s has no default", name)
		}
	}
}
