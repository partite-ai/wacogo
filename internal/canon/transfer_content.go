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
	if byteLen > maxStringByteLength {
		panic(&Trap{msg: "string byte length exceeds max"})
	}
	mustTransfer(checkAlignment(srcPtr, stringAlign(srcEnc)))

	srcBytes, ok := tc.caller.Memory.Read(srcPtr, byteLen)
	if !ok {
		panic(&Trap{msg: fmt.Sprintf("string content out-of-bounds: ptr=%d len=%d", srcPtr, byteLen)})
	}

	var outBytes []byte
	var outCoded uint32
	if srcEnc == dstEnc {
		switch srcEnc {
		case EncUTF8:
			if msg := validateUTF8(srcBytes); msg != "" {
				panic(&Trap{msg: msg})
			}
			outBytes, outCoded = srcBytes, srcCoded
		case EncUTF16:
			validateUTF16Bytes(srcBytes)
			outBytes, outCoded = srcBytes, srcCoded
		case EncLatin1UTF16:
			// Per spec, the Latin-1 sub-encoding has no validation work to
			// do (every byte is a valid code point); the UTF-16 sub-encoding
			// validates surrogate pairing.
			if _, isUTF16 := latin1UTF16Decode(srcCoded); isUTF16 {
				validateUTF16Bytes(srcBytes)
			}
			outBytes, outCoded = srcBytes, srcCoded
		default:
			outBytes, outCoded = srcBytes, srcCoded
		}
	} else {
		s := decodeString(memShim{tc.caller.Memory}, srcPtr, srcCoded, srcEnc)
		outBytes, outCoded = encodeString(s, dstEnc)
	}

	// The discovery walk requested a buffer sized to the worst-case under
	// stringEncoderUpperBound. The actual output length (len(outBytes)) is
	// always ≤ that bound. The destination pointer reserves at least that
	// many bytes; we only write the actual outBytes.
	dstPtr := src.next(ctx, tc, uint32(len(outBytes)), stringAlign(dstEnc))
	if !tc.callee.Memory.Write(dstPtr, outBytes) {
		panic(&Trap{msg: "string oob write"})
	}
	return dstPtr, outCoded
}

// stringContentSizeStep is the discovery walk for a string transfer.
// Given the source coded length, it computes the upper-bound byte count
// under the applicable encoding conversion and reports a single allocation.
// Source bytes themselves are not needed for sizing.
func stringContentSizeStep(tc *transferContext, srcCoded uint32, sink *allocSink) {
	srcEnc := tc.caller.StringEncoding
	dstEnc := tc.callee.StringEncoding
	byteLen := stringByteLen(srcCoded, srcEnc)
	upper := byteLen * stringEncoderUpperBoundMultiplier(srcEnc, dstEnc)
	sink.add(upper, stringAlign(dstEnc))
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
