package canon

import (
	"context"
	"fmt"
)

// transferStringContent validates the caller's string payload, re-encodes
// into the callee's encoding if needed, reallocates a buffer in callee memory,
// and writes the bytes. It returns the (dstPtr, outCoded) pair — callers are
// responsible for reading srcPtr/srcCoded from their side-specific slot or
// byte offset and writing the returned pair back the same way.
func transferStringContent(ctx context.Context, tc *transferContext, srcPtr, srcCoded uint32) (uint32, uint32) {
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
			if msg := classifyUTF8(srcBytes); msg != "" {
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

	dstPtr, err := callRealloc(ctx, tc.callee.Realloc, tc.callee.Memory, 0, 0, stringAlign(dstEnc), uint32(len(outBytes)))
	if err != nil {
		panic(&Trap{msg: err.Error()})
	}
	if !tc.callee.Memory.Write(dstPtr, outBytes) {
		panic(&Trap{msg: "string oob write"})
	}
	return dstPtr, outCoded
}

// transferListContent validates the caller's list header, reallocates an
// element buffer in callee memory, and runs subSteps once per element,
// passing per-element src/dst bases directly as step args. Returns
// (dstPtr, n) for the caller to emit back to its side-specific output.
func transferListContent(
	ctx context.Context,
	tc *transferContext,
	srcPtr, n, elemSize, elemAlign uint32,
	subSteps []transferPlanStep,
) (uint32, uint32) {
	mustTransfer(checkListLen(n, elemSize))
	if n > 0 {
		mustTransfer(checkAlignment(srcPtr, elemAlign))
	}

	dstPtr, err := callRealloc(ctx, tc.callee.Realloc, tc.callee.Memory, 0, 0, elemAlign, n*elemSize)
	if err != nil {
		panic(&Trap{msg: err.Error()})
	}

	for i := range n {
		elemSrc := srcPtr + i*elemSize
		elemDst := dstPtr + i*elemSize
		for _, step := range subSteps {
			step(ctx, tc, elemSrc, elemDst)
		}
	}

	return dstPtr, n
}
