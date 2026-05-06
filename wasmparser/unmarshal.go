package wasmparser

// binaryUnmarshaler is implemented by types that can decode themselves from a BinaryReader.
type binaryUnmarshaler interface {
	unmarshalBinary(r *BinaryReader) error
}

// unmarshalNew instantiates a new T, calls unmarshalBinary on it, and returns it.
// T must be a pointer type whose base type implements binaryUnmarshaler.
func unmarshalNew[T interface {
	binaryUnmarshaler
	~*E
}, E any](r *BinaryReader) (T, error) {
	t := T(new(E))
	err := t.unmarshalBinary(r)
	return t, err
}
