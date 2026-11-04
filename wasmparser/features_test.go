package wasmparser

import (
	"bytes"
	"io"
	"testing"
)

func TestFeatureSet(t *testing.T) {
	fs := DefaultFeatures()
	if !fs.Has(FeatureComponentModel) {
		t.Fatal("default features should include ComponentModel")
	}
	if fs.Has(FeatureAsync) {
		t.Fatal("default features should not include Async")
	}

	all := AllFeatures()
	if !all.Has(FeatureAsync) {
		t.Fatal("all features should include Async")
	}
	if !all.Has(FeatureThreads) {
		t.Fatal("all features should include Threads")
	}
}

// A feature gate only applies when the feature is off.
func TestFeatureGates(t *testing.T) {
	for name, tc := range map[string]struct {
		feature Feature
		wat     string
	}{
		"error-context type":  {FeatureErrorContext, `(component (type (list error-context)))`},
		"error-context param": {FeatureErrorContext, `(component (import "f" (func (param "x" error-context))))`},
		"value import":        {FeatureValues, `(component (import "v" (value $v string)) (export "e" (value $v)))`},
		"start": {FeatureValues, `(component
  (core module $m (func (export "g")))
  (core instance $i (instantiate $m))
  (func $s (canon lift (core func $i "g")))
  (start $s))`},
	} {
		t.Run(name, func(t *testing.T) {
			validate := func(fs FeatureSet) error {
				vp := NewValidatingParser(bytes.NewReader(wat2wasm(t, tc.wat)), fs)
				for {
					if _, err := vp.Next(); err == io.EOF {
						return nil
					} else if err != nil {
						return err
					}
				}
			}
			if err := validate(DefaultFeatures()); err == nil {
				t.Fatal("want rejected by default")
			}
			enabled := DefaultFeatures()
			enabled[tc.feature] = struct{}{}
			if err := validate(enabled); err != nil {
				t.Fatalf("want accepted with its feature on: %v", err)
			}
		})
	}
}
