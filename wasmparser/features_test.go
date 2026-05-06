package wasmparser

import "testing"

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
