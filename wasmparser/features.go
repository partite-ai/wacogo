package wasmparser

// Feature represents a WebAssembly feature that can be enabled or disabled.
type Feature uint32

const (
	FeatureComponentModel Feature = iota
	FeatureAsync
	FeatureThreads
	FeatureValues
	FeatureErrorContext
	FeatureGC
	FeatureNestedNames
	FeatureFixedLengthLists
	featureCount
)

// FeatureSet tracks which features are enabled.
type FeatureSet map[Feature]struct{}

// DefaultFeatures returns a FeatureSet with only MVP features enabled.
func DefaultFeatures() FeatureSet {
	return FeatureSet{FeatureComponentModel: {}}
}

// AllFeatures returns a FeatureSet with all features enabled.
func AllFeatures() FeatureSet {
	fs := make(FeatureSet)
	for i := range featureCount {
		fs[i] = struct{}{}
	}
	return fs
}

// Has returns whether the given feature is enabled.
func (fs FeatureSet) Has(f Feature) bool {
	_, ok := fs[f]
	return ok
}
