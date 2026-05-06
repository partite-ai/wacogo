package wasmparser

// Optional represents a value that may or may not be present.
type Optional[T any] struct {
	Value T
	Valid bool
}

// Some creates an Optional with a present value.
func Some[T any](v T) Optional[T] {
	return Optional[T]{Value: v, Valid: true}
}

// None creates an Optional with no value.
func None[T any]() Optional[T] {
	return Optional[T]{}
}
