package wasmparser

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"unicode/utf8"
)

// BinaryReader reads WebAssembly binary data with offset tracking.
// It wraps a bufio.Reader for buffered streaming reads.
type BinaryReader struct {
	r      *bufio.Reader
	offset uint64 // current position in the original stream (for error reporting)
	// nesting tracks recursive parsing depth (nested components/modules,
	// recursive type declarations). Bounded by MaxNestingDepth.
	nesting int
}

// NewBinaryReader creates a BinaryReader that reads from r.
// For byte slices, use NewBinaryReader(bytes.NewReader(data)).
// For bounded reads, use NewBinaryReader(io.LimitReader(r, n)).
func NewBinaryReader(r io.Reader) *BinaryReader {
	return &BinaryReader{r: bufio.NewReader(r)}
}

// newBinaryReaderAt creates a BinaryReader with a base offset for error reporting.
// Used internally for sub-parsers and section readers where the data originates
// at a known position in the original stream.
func newBinaryReaderAt(r io.Reader, offset uint64) *BinaryReader {
	return &BinaryReader{r: bufio.NewReader(r), offset: offset}
}

// Offset returns the current stream position (base offset + bytes consumed).
func (r *BinaryReader) Offset() uint64 {
	return r.offset
}

// IsEOF returns true if the reader has no more data.
func (r *BinaryReader) IsEOF() bool {
	_, err := r.r.Peek(1)
	return err != nil
}

// Peek returns the next byte without advancing the position.
func (r *BinaryReader) Peek() (byte, error) {
	bs, err := r.r.Peek(1)
	if err != nil {
		if err == io.EOF {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, err
	}
	return bs[0], nil
}

// ReadByte reads and returns the next byte.
func (r *BinaryReader) ReadByte() (byte, error) {
	b, err := r.r.ReadByte()
	if err != nil {
		if err == io.EOF {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, err
	}
	r.offset++
	return b, nil
}

// ReadBytes reads exactly n bytes and returns them. Returns io.ErrUnexpectedEOF if insufficient data.
func (r *BinaryReader) ReadBytes(n int) ([]byte, error) {
	if n == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r.r, buf)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	r.offset += uint64(n)
	return buf, nil
}

// ---- LEB128 decoding ----

// ReadU32 decodes an unsigned LEB128-encoded 32-bit integer.
func (r *BinaryReader) ReadU32() (uint32, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	if b&0x80 == 0 {
		return uint32(b), nil
	}
	result := uint32(b & 0x7F)
	shift := uint(7)
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint32(b&0x7F) << shift
		if shift >= 25 {
			// At shift=28 we've consumed 5 bytes. Check for overflow.
			// The remaining valid bits are (32 - shift) bits from b.
			if b>>uint(32-shift) != 0 {
				if b&0x80 != 0 {
					return 0, errfAt(r.Offset(), "LEB128 integer too long")
				}
				return 0, errfAt(r.Offset(), "LEB128 integer too large for u32")
			}
		}
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	return result, nil
}

// ReadU64 decodes an unsigned LEB128-encoded 64-bit integer.
func (r *BinaryReader) ReadU64() (uint64, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	if b&0x80 == 0 {
		return uint64(b), nil
	}
	result := uint64(b & 0x7F)
	shift := uint(7)
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint64(b&0x7F) << shift
		if shift >= 57 {
			if b>>uint(64-shift) != 0 {
				if b&0x80 != 0 {
					return 0, errfAt(r.Offset(), "LEB128 integer too long")
				}
				return 0, errfAt(r.Offset(), "LEB128 integer too large for u64")
			}
		}
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	return result, nil
}

// ReadS32 decodes a signed LEB128-encoded 32-bit integer.
func (r *BinaryReader) ReadS32() (int32, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	if b&0x80 == 0 {
		// Sign-extend the 7-bit value
		return int32(int8(b<<1)) >> 1, nil
	}
	result := int32(b & 0x7F)
	shift := uint(7)
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= int32(b&0x7F) << shift
		if shift >= 25 {
			// Check for continuation bit and sign overflow
			signBits := int8(b<<1) >> (32 - shift)
			if b&0x80 != 0 || (signBits != 0 && signBits != -1) {
				if b&0x80 != 0 {
					return 0, errfAt(r.Offset(), "LEB128 integer too long")
				}
				return 0, errfAt(r.Offset(), "LEB128 integer too large for s32")
			}
			return result, nil
		}
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	// Sign extend
	ashift := uint(32 - shift)
	return (result << ashift) >> ashift, nil
}

// ReadS33 decodes a signed LEB128-encoded 33-bit integer (returned as int64).
// Used by the Component Model for type index encoding.
func (r *BinaryReader) ReadS33() (int64, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	if b&0x80 == 0 {
		// Sign-extend the 7-bit value to 64 bits
		return int64(int8(b<<1)) >> 1, nil
	}
	result := int64(b & 0x7F)
	shift := uint(7)
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= int64(b&0x7F) << shift
		if shift >= 25 {
			// sign check uses 33-shift for the 33-bit boundary
			signBits := int8(b<<1) >> (33 - shift)
			if b&0x80 != 0 || (signBits != 0 && signBits != -1) {
				if b&0x80 != 0 {
					return 0, errfAt(r.Offset(), "LEB128 integer too long")
				}
				return 0, errfAt(r.Offset(), "LEB128 integer too large for s33")
			}
			// At this threshold (shift=28), we've consumed all 33 bits.
			// Sign-extend from bit 32 (the 33rd bit) to 64 bits.
			const s33Width = 33
			ashift := uint(64 - s33Width)
			return (result << ashift) >> ashift, nil
		}
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	// Non-threshold exit: sign-extend from bit (shift-1).
	// shift has been incremented, so sign bit is at position shift-1.
	ashift := uint(64 - shift)
	return (result << ashift) >> ashift, nil
}

// ReadS64 decodes a signed LEB128-encoded 64-bit integer.
func (r *BinaryReader) ReadS64() (int64, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	if b&0x80 == 0 {
		return int64(int8(b<<1)) >> 1, nil
	}
	result := int64(b & 0x7F)
	shift := uint(7)
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= int64(b&0x7F) << shift
		if shift >= 57 {
			signBits := int8(b<<1) >> (64 - shift)
			if b&0x80 != 0 || (signBits != 0 && signBits != -1) {
				if b&0x80 != 0 {
					return 0, errfAt(r.Offset(), "LEB128 integer too long")
				}
				return 0, errfAt(r.Offset(), "LEB128 integer too large for s64")
			}
			return result, nil
		}
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	ashift := uint(64 - shift)
	return (result << ashift) >> ashift, nil
}

// ---- Strings and Floats ----

// ReadString reads a length-prefixed UTF-8 string.
// The length is a u32 LEB128 value. Returns an error if the string exceeds
// MaxStringSize or contains invalid UTF-8.
func (r *BinaryReader) ReadString() (string, error) {
	length, err := r.ReadU32()
	if err != nil {
		return "", err
	}
	if length > MaxStringSize {
		return "", errfAt(r.Offset(), "string length %d exceeds maximum %d", length, MaxStringSize)
	}
	bs, err := r.ReadBytes(int(length))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(bs) {
		return "", errAt(r.Offset(), "string is not valid UTF-8")
	}
	return string(bs), nil
}

// ReadF32 reads a 32-bit IEEE 754 float in little-endian byte order.
func (r *BinaryReader) ReadF32() (float32, error) {
	bs, err := r.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	bits := binary.LittleEndian.Uint32(bs)
	return math.Float32frombits(bits), nil
}

// ReadF64 reads a 64-bit IEEE 754 float in little-endian byte order.
func (r *BinaryReader) ReadF64() (float64, error) {
	bs, err := r.ReadBytes(8)
	if err != nil {
		return 0, err
	}
	bits := binary.LittleEndian.Uint64(bs)
	return math.Float64frombits(bits), nil
}
