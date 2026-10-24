package canon

import (
	"unicode/utf16"
	"unicode/utf8"
)

// validateUTF8 is the fast path for UTF-8 validation: returns "" if b is
// valid UTF-8, otherwise the precise canon-ABI trap message. utf8.Valid is
// table-driven and substantially faster than the hand-rolled scalar loop
// in classifyUTF8, especially on long ASCII strings; classifyUTF8 runs only
// on failure to recover the spec-mandated trap message.
func validateUTF8(b []byte) string {
	if utf8.Valid(b) {
		return ""
	}
	return classifyUTF8(b)
}

// classifyUTF8 returns "" if b is valid UTF-8, otherwise a short message
// distinguishing a truncated trailing sequence ("incomplete utf-8 byte
// sequence") from any other error ("invalid utf-8"). Slow path — prefer
// validateUTF8 unless you already know the input is invalid.
func classifyUTF8(b []byte) string {
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			i++
			continue
		}
		var n int
		switch {
		case c >= 0xC2 && c <= 0xDF:
			n = 2
		case c >= 0xE0 && c <= 0xEF:
			n = 3
		case c >= 0xF0 && c <= 0xF4:
			n = 4
		default:
			return "invalid utf-8"
		}
		if i+n > len(b) {
			return "incomplete utf-8 byte sequence"
		}
		r, size := utf8.DecodeRune(b[i : i+n])
		if r == utf8.RuneError && size == 1 {
			return "invalid utf-8"
		}
		i += size
	}
	return ""
}

const maxStringByteLength uint32 = (1 << 28) - 1

// latin1UTF16HighBit is the canonical-ABI tag bit on a Latin-1+UTF-16
// codedLen: clear ⇒ Latin-1 (1 byte/char), set ⇒ UTF-16 (2 bytes/unit).
const latin1UTF16HighBit uint32 = 1 << 31

func unitSize(enc StringEncoding) uint32 {
	switch enc {
	case EncUTF8, EncLatin1UTF16:
		return 1
	case EncUTF16:
		return 2
	}
	return 1
}

func stringAlign(enc StringEncoding) uint32 {
	switch enc {
	case EncUTF16, EncLatin1UTF16:
		return 2
	}
	return 1
}

// latin1UTF16Decode interprets a codedLen field for Latin-1+UTF-16 encoding.
// effLen is the count without the tag bit. isUTF16 selects between the two
// sub-encodings.
func latin1UTF16Decode(codedLen uint32) (effLen uint32, isUTF16 bool) {
	if codedLen&latin1UTF16HighBit != 0 {
		return codedLen &^ latin1UTF16HighBit, true
	}
	return codedLen, false
}

// stringByteLen returns the in-memory byte length for a (codedLen, enc) pair.
// For Latin-1+UTF-16, the high tag bit selects between the two sub-encodings.
// Computed in 64 bits so a UTF-16 code-unit count cannot wrap; callers
// compare against maxStringByteLength before narrowing.
func stringByteLen(codedLen uint32, enc StringEncoding) uint64 {
	switch enc {
	case EncUTF8:
		return uint64(codedLen)
	case EncUTF16:
		return uint64(codedLen) * 2
	case EncLatin1UTF16:
		eff, isUTF16 := latin1UTF16Decode(codedLen)
		if isUTF16 {
			return uint64(eff) * 2
		}
		return uint64(eff)
	}
	return 0
}

// readStringBytes returns the raw bytes of a source string after checking
// its encoding, length cap, alignment, and bounds. Traps on any failure.
// The returned slice aliases mem.
func readStringBytes(mem fakeOrRealMemory, ptr, codedLen uint32, enc StringEncoding) []byte {
	switch enc {
	case EncUTF8, EncUTF16, EncLatin1UTF16:
	default:
		trapf("unknown string encoding")
	}
	byteLen := stringByteLen(codedLen, enc)
	if byteLen > uint64(maxStringByteLength) {
		trapf("string byte length %d exceeds max %d", byteLen, maxStringByteLength)
	}
	if ptr%stringAlign(enc) != 0 {
		trapf("unaligned pointer %d for string encoding (align %d)", ptr, stringAlign(enc))
	}
	bytes, ok := mem.Read(ptr, uint32(byteLen))
	if !ok {
		trapf("string pointer/length out of bounds of memory: ptr=%d len=%d", ptr, byteLen)
	}
	return bytes
}

func decodeString(mem fakeOrRealMemory, ptr, codedLen uint32, enc StringEncoding) string {
	bytes := readStringBytes(mem, ptr, codedLen, enc)
	switch enc {
	case EncUTF8:
		if msg := validateUTF8(bytes); msg != "" {
			trapf("%s", msg)
		}
		return string(bytes)
	case EncUTF16:
		return decodeUTF16(bytes)
	case EncLatin1UTF16:
		if _, isUTF16 := latin1UTF16Decode(codedLen); isUTF16 {
			return decodeUTF16(bytes)
		}
		return decodeLatin1(bytes)
	}
	trapf("unknown string encoding")
	return ""
}

// decodeLatin1 decodes ISO-8859-1 (each byte = code point) into a Go string.
// Every byte is a valid code point so no validation is needed.
func decodeLatin1(bytes []byte) string {
	runes := make([]rune, len(bytes))
	for i, b := range bytes {
		runes[i] = rune(b)
	}
	return string(runes)
}

func decodeUTF16(bytes []byte) string {
	if len(bytes)%2 != 0 {
		trapf("UTF-16 byte length must be even")
	}
	units := make([]uint16, len(bytes)/2)
	for i := range units {
		units[i] = uint16(bytes[2*i]) | uint16(bytes[2*i+1])<<8
	}
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xD800 && u <= 0xDBFF {
			if i+1 >= len(units) {
				trapf("unpaired high surrogate at %d", i)
			}
			low := units[i+1]
			if low < 0xDC00 || low > 0xDFFF {
				trapf("high surrogate not followed by low surrogate")
			}
			i++
		} else if u >= 0xDC00 && u <= 0xDFFF {
			trapf("unpaired low surrogate at %d", i)
		}
	}
	runes := utf16.Decode(units)
	return string(runes)
}

type fakeOrRealMemory interface {
	Read(byteOff, length uint32) ([]byte, bool)
}

func encodeString(s string, enc StringEncoding) ([]byte, uint32) {
	switch enc {
	case EncUTF8:
		b := []byte(s)
		return b, uint32(len(b))
	case EncUTF16:
		units := utf16.Encode([]rune(s))
		b := make([]byte, 2*len(units))
		for i, u := range units {
			b[2*i] = byte(u)
			b[2*i+1] = byte(u >> 8)
		}
		return b, uint32(len(units))
	case EncLatin1UTF16:
		// Attempt Latin-1: every rune must fit in [0, 0xFF].
		runes := []rune(s)
		latin1 := true
		for _, r := range runes {
			if r > 0xFF {
				latin1 = false
				break
			}
		}
		if latin1 {
			b := make([]byte, len(runes))
			for i, r := range runes {
				b[i] = byte(r)
			}
			// High bit clear: this is Latin-1 (1 byte/char).
			return b, uint32(len(b))
		}
		// Fall back to UTF-16 with the high tag bit set.
		units := utf16.Encode(runes)
		b := make([]byte, 2*len(units))
		for i, u := range units {
			b[2*i] = byte(u)
			b[2*i+1] = byte(u >> 8)
		}
		return b, uint32(len(units)) | latin1UTF16HighBit
	}
	trapf("unknown encoding")
	return nil, 0
}

// validateUTF16Bytes traps if bytes is not a well-formed UTF-16LE sequence.
// Does not allocate — surrogate pairing is checked in-place over the input.
func validateUTF16Bytes(bytes []byte) {
	if len(bytes)%2 != 0 {
		trapf("UTF-16 byte length must be even")
	}
	n := len(bytes) / 2
	for i := 0; i < n; i++ {
		u := uint16(bytes[2*i]) | uint16(bytes[2*i+1])<<8
		if u >= 0xD800 && u <= 0xDBFF {
			if i+1 >= n {
				trapf("unpaired high surrogate at %d", i)
			}
			low := uint16(bytes[2*(i+1)]) | uint16(bytes[2*(i+1)+1])<<8
			if low < 0xDC00 || low > 0xDFFF {
				trapf("high surrogate not followed by low surrogate")
			}
			i++
		} else if u >= 0xDC00 && u <= 0xDFFF {
			trapf("unpaired low surrogate at %d", i)
		}
	}
}

// transcodedByteLen returns the exact byte length src will occupy once
// transcoded from srcEnc to dstEnc, matching what encodeString produces
// for the decoded string. src must be the bytes returned by
// readStringBytes for (srcCoded, srcEnc). Traps if src is malformed in
// srcEnc. Does not allocate.
func transcodedByteLen(src []byte, srcCoded uint32, srcEnc, dstEnc StringEncoding) uint32 {
	srcUTF16 := srcEnc == EncUTF16
	if srcEnc == EncLatin1UTF16 {
		_, srcUTF16 = latin1UTF16Decode(srcCoded)
	}
	if srcEnc == EncUTF8 {
		if msg := validateUTF8(src); msg != "" {
			trapf("%s", msg)
		}
	} else if srcUTF16 {
		validateUTF16Bytes(src)
	}

	switch dstEnc {
	case EncUTF8:
		return utf8LenOf(src, srcEnc, srcUTF16)
	case EncUTF16:
		return utf16LenOf(src, srcEnc, srcUTF16)
	case EncLatin1UTF16:
		if fitsLatin1(src, srcEnc, srcUTF16) {
			// One byte per code point.
			if srcEnc == EncUTF8 {
				return uint32(utf8.RuneCount(src))
			}
			if srcUTF16 {
				return uint32(len(src) / 2)
			}
			return uint32(len(src))
		}
		return utf16LenOf(src, srcEnc, srcUTF16)
	}
	trapf("unknown string encoding")
	return 0
}

// utf8LenOf returns the UTF-8 byte length of a validated source string.
func utf8LenOf(src []byte, srcEnc StringEncoding, srcUTF16 bool) uint32 {
	switch {
	case srcEnc == EncUTF8:
		return uint32(len(src))
	case srcUTF16:
		var n uint32
		for i := 0; i+1 < len(src); i += 2 {
			u := uint16(src[i]) | uint16(src[i+1])<<8
			switch {
			case u < 0x80:
				n++
			case u < 0x800:
				n += 2
			case u >= 0xD800 && u <= 0xDBFF:
				// High surrogate: the pair encodes one supplementary
				// code point, 4 UTF-8 bytes. Validation guarantees the
				// low surrogate follows.
				n += 4
				i += 2
			default:
				n += 3
			}
		}
		return n
	default: // Latin-1
		var n uint32
		for _, b := range src {
			if b < 0x80 {
				n++
			} else {
				n += 2
			}
		}
		return n
	}
}

// utf16LenOf returns the UTF-16 byte length of a validated source string.
func utf16LenOf(src []byte, srcEnc StringEncoding, srcUTF16 bool) uint32 {
	switch {
	case srcEnc == EncUTF8:
		var n uint32
		for i := 0; i < len(src); {
			r, size := utf8.DecodeRune(src[i:])
			if r >= 0x10000 {
				n += 4
			} else {
				n += 2
			}
			i += size
		}
		return n
	case srcUTF16:
		return uint32(len(src))
	default: // Latin-1
		return uint32(len(src)) * 2
	}
}

// fitsLatin1 reports whether every code point of a validated source string
// is in [0, 0xFF].
func fitsLatin1(src []byte, srcEnc StringEncoding, srcUTF16 bool) bool {
	switch {
	case srcEnc == EncUTF8:
		// U+0000..U+00FF encode as ASCII or a 2-byte sequence led by
		// 0xC2/0xC3; any other byte ≥ 0x80 that is a leading byte means
		// a code point above U+00FF.
		for _, b := range src {
			if b >= 0xC4 {
				return false
			}
		}
		return true
	case srcUTF16:
		for i := 1; i < len(src); i += 2 {
			if src[i] != 0 {
				return false
			}
		}
		return true
	default: // Latin-1
		return true
	}
}
