package canon

import (
	"context"
	"fmt"
)

// allocSource is the runtime view of a sequence of pre-allocated callee-side
// pointers, consumed in declaration order by transfer steps. A single
// concrete (not interface) type with two modes — chosen by checking helper
// — keeps the per-call cost of `src.next` to a direct method dispatch with
// no interface boxing.
//
//   - helper == nil: serial mode. next calls callRealloc directly per
//     request, behaviour identical to the pre-batching code path.
//   - helper != nil: batched mode. The discovery pass has already
//     populated helper scratch with N (size, align) entries and helper
//     has issued N reallocs; next reads pointers out in order.
type allocSource struct {
	helper *batchReallocHelper
	n      uint32 // pre-allocated count in batched mode; 0 in serial
	idx    uint32 // batched cursor
}

func (s *allocSource) next(ctx context.Context, tc *transferContext, size, align uint32) uint32 {
	if s.helper == nil {
		// Serial fallback: directly invoke target's cabi_realloc.
		p, err := callRealloc(ctx, tc.callee.Realloc, tc.callee.Memory, 0, 0, align, size)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		return p
	}
	if s.idx >= s.n {
		panic(fmt.Sprintf("allocSource: cursor %d >= count %d (size discovery missed an alloc)", s.idx, s.n))
	}
	p := s.helper.readPtr(s.n, s.idx)
	s.idx++
	return p
}

// allocSink records each (size, align) request during the discovery walk.
// Backed by direct writes into batch helper scratch — no Go-side slice
// allocation. The runner reads sink.n after the walk to know how many
// allocations to ask the helper to perform. Scratch is grown on demand
// when N exceeds the initial 1-page capacity.
type allocSink struct {
	helper *batchReallocHelper
	n      uint32
}

func (s *allocSink) add(size, align uint32) {
	s.helper.ensureRoomFor(s.n + 1)
	s.helper.writeSizeAlign(s.n, size, align)
	s.n++
}
