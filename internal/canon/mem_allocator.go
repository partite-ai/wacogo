package canon

import "github.com/tetratelabs/wazero/experimental"

// internalMemoryAllocator backs the linear memory of wacogo's internal
// helper modules (import/re-export stubs and batch-realloc helpers) with
// plain Go slices.
//
// The engine's caller may install a custom, mmap-backed MemoryAllocator for
// guest modules. That allocator rides in on the instantiation context and
// would otherwise be applied to these tiny, frequently-instantiated internal
// modules too, where per-instance mmap is pure overhead. We override it back
// to the cheap slice path at the point we instantiate them. (The context key
// cannot be cleared — WithMemoryAllocator(ctx, nil) is a no-op and the key is
// internal to wazero — so we override rather than unset.)
var internalMemoryAllocator = experimental.MemoryAllocatorFunc(
	func(_, _ uint64) experimental.LinearMemory { return &sliceMemory{} },
)

// sliceMemory is a LinearMemory backed by a single Go slice. It is valid only
// for non-shared memories — the helper modules never declare shared memory —
// so Reallocate is free to move the backing array; wazero reloads the memory
// base after each grow.
type sliceMemory struct {
	buf []byte
}

// Reallocate returns a buffer of exactly size bytes, growing (and moving) the
// backing array when the current capacity is too small.
func (m *sliceMemory) Reallocate(size uint64) []byte {
	if uint64(cap(m.buf)) < size {
		next := make([]byte, size)
		copy(next, m.buf)
		m.buf = next
	} else {
		m.buf = m.buf[:size]
	}
	return m.buf
}

// Free releases the backing slice.
func (m *sliceMemory) Free() { m.buf = nil }
