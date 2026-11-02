package canon

import (
	"context"
	"fmt"
)

// transferStringContent validates the caller's string payload, re-encodes
// into the callee's encoding if needed, obtains a buffer in callee memory
// via src.next, and writes the bytes. Returns (dstPtr, outCoded).
func transferStringContent(ctx context.Context, tc *transferContext, srcPtr, srcCoded uint32, src *allocSource) (uint32, uint32) {
	srcEnc := tc.caller.StringEncoding
	dstEnc := tc.callee.StringEncoding

	byteLen := stringByteLen(srcCoded, srcEnc)
	if byteLen > uint64(maxStringByteLength) {
		panic(&Trap{msg: "string byte length exceeds max"})
	}
	mustTransfer(checkAlignment(srcPtr, stringAlign(srcEnc)))

	srcBytes, ok := tc.caller.Memory.Read(srcPtr, uint32(byteLen))
	if !ok {
		panic(&Trap{msg: fmt.Sprintf("string content out-of-bounds: ptr=%d len=%d", srcPtr, byteLen)})
	}

	if srcEnc != dstEnc {
		return transcodeStringContent(ctx, tc, srcBytes, srcCoded, src)
	}
	switch srcEnc {
	case EncUTF8:
		if msg := validateUTF8(srcBytes); msg != "" {
			panic(&Trap{msg: msg})
		}
	case EncUTF16:
		validateUTF16Bytes(srcBytes)
	case EncLatin1UTF16:
		// Per spec, the Latin-1 sub-encoding has no validation work to
		// do (every byte is a valid code point); the UTF-16 sub-encoding
		// validates surrogate pairing.
		if _, isUTF16 := latin1UTF16Decode(srcCoded); isUTF16 {
			validateUTF16Bytes(srcBytes)
		}
	}
	// In batched mode the discovery walk (stringContentSizeStep) already
	// reserved exactly len(srcBytes); src.next checks the sizes agree.
	dstPtr := src.next(ctx, tc, uint32(len(srcBytes)), stringAlign(dstEnc))
	if !tc.callee.Memory.Write(dstPtr, srcBytes) {
		panic(&Trap{msg: "string oob write"})
	}
	return dstPtr, srcCoded
}

// transcodeStringContent transfers a string between differing encodings.
// It allocates the exact transcoded size (which the discovery walk
// reserved in batched mode, and which src.next charges to the lift budget
// in serial mode) before transcoding, then transcodes straight into the
// callee's buffer, so a hostile length is refused before any work and no
// Go-side copy of the string is made.
func transcodeStringContent(ctx context.Context, tc *transferContext, srcBytes []byte, srcCoded uint32, src *allocSource) (uint32, uint32) {
	srcEnc := tc.caller.StringEncoding
	dstEnc := tc.callee.StringEncoding
	size, reserved := src.reservedSize()
	if !reserved {
		size = transcodedByteLen(srcBytes, srcCoded, srcEnc, dstEnc)
	}
	dstPtr := src.next(ctx, tc, size, stringAlign(dstEnc))
	dst, ok := tc.callee.Memory.Read(dstPtr, size)
	if !ok {
		panic(&Trap{msg: "string oob write"})
	}
	return dstPtr, transcodeInto(dst, srcBytes, srcCoded, srcEnc, dstEnc)
}

// stringContentSizeStep is the discovery walk for a string transfer. It
// reports a single allocation of exactly the byte length the transfer step
// will write. Same-encoding transfers copy verbatim, so the size follows
// from srcCoded alone; cross-encoding transfers read and validate the
// source to count its transcoded length.
func stringContentSizeStep(tc *transferContext, srcPtr, srcCoded uint32, sink *allocSink) {
	srcEnc := tc.caller.StringEncoding
	dstEnc := tc.callee.StringEncoding
	if srcEnc == dstEnc {
		byteLen := stringByteLen(srcCoded, srcEnc)
		if byteLen > uint64(maxStringByteLength) {
			panic(&Trap{msg: "string byte length exceeds max"})
		}
		sink.add(uint32(byteLen), stringAlign(dstEnc))
		return
	}
	srcBytes := readStringBytes(memShim{tc.caller.Memory}, srcPtr, srcCoded, srcEnc)
	sink.add(transcodedByteLen(srcBytes, srcCoded, srcEnc, dstEnc), stringAlign(dstEnc))
}

// transferListBulk is the fast path for lists whose element type's
// transfer is a pure byte pass-through (no validation, encoding change,
// or resource handle translation). It validates the list header, obtains
// the destination buffer via src.next, and performs a single bulk
// Read/Write of the entire n*elemSize byte span.
//
// The visitor (memTransferVisitor.VisitList / flatTransferVisitor.VisitList)
// selects this path when child.byteEquivalent is true.
func transferListBulk(ctx context.Context, tc *transferContext, srcPtr, n, elemSize, elemAlign uint32, src *allocSource) (uint32, uint32) {
	mustTransfer(checkListLen(n, elemSize))
	if n > 0 {
		mustTransfer(checkAlignment(srcPtr, elemAlign))
	}

	totalSize := n * elemSize
	dstPtr := src.next(ctx, tc, totalSize, elemAlign)
	if totalSize > 0 {
		srcBytes, ok := tc.caller.Memory.Read(srcPtr, totalSize)
		if !ok {
			panic(&Trap{msg: fmt.Sprintf("list bulk read out-of-bounds: ptr=%d len=%d", srcPtr, totalSize)})
		}
		if !tc.callee.Memory.Write(dstPtr, srcBytes) {
			panic(&Trap{msg: "list bulk write out-of-bounds"})
		}
	}
	return dstPtr, n
}

// transferListContent validates the caller's list header, obtains the
// element buffer via src.next, and runs subSteps once per element. Each
// element's transfer pulls its own allocations from src in turn.
func transferListContent(
	ctx context.Context,
	tc *transferContext,
	srcPtr, n, elemSize, elemAlign uint32,
	subSteps []transferPlanStep,
	src *allocSource,
) (uint32, uint32) {
	mustTransfer(checkListLen(n, elemSize))
	if n > 0 {
		mustTransfer(checkAlignment(srcPtr, elemAlign))
	}

	dstPtr := src.next(ctx, tc, n*elemSize, elemAlign)

	for i := range n {
		elemSrc := srcPtr + i*elemSize
		elemDst := dstPtr + i*elemSize
		for _, step := range subSteps {
			step.transfer(ctx, tc, elemSrc, elemDst, src)
		}
	}

	return dstPtr, n
}

// listContentSizes is the discovery walk for transferListContent: it adds
// the buffer-allocation entry, then recurses per element by invoking each
// subStep's size walk if non-nil. Order matches the transfer walk above.
func listContentSizes(
	tc *transferContext,
	srcPtr, n, elemSize, elemAlign uint32,
	subSteps []transferPlanStep,
	sink *allocSink,
) {
	sink.add(n*elemSize, elemAlign)
	for i := uint32(0); i < n; i++ {
		elemSrc := srcPtr + i*elemSize
		for _, step := range subSteps {
			if step.sizes != nil {
				step.sizes(tc, elemSrc, sink)
			}
		}
	}
}

// listBulkSizes is the discovery walk for transferListBulk: a single
// buffer allocation entry for n*elemSize bytes.
func listBulkSizes(_ *transferContext, n, elemSize, elemAlign uint32, sink *allocSink) {
	sink.add(n*elemSize, elemAlign)
}
