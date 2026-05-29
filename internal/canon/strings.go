package canon

import (
	"unicode/utf16"
	"unicode/utf8"
)

// classifyUTF8 returns "" if b is valid UTF-8, otherwise a short message
// distinguishing a truncated trailing sequence ("incomplete utf-8 byte
// sequence") from any other error ("invalid utf-8").
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
func stringByteLen(codedLen uint32, enc StringEncoding) uint32 {
	switch enc {
	case EncUTF8:
		return codedLen
	case EncUTF16:
		return codedLen * 2
	case EncLatin1UTF16:
		eff, isUTF16 := latin1UTF16Decode(codedLen)
		if isUTF16 {
			return eff * 2
		}
		return eff
	}
	return 0
}

func decodeString(mem fakeOrRealMemory, ptr, codedLen uint32, enc StringEncoding) string {
	// Compute byteLen with explicit overflow check for the multiplication.
	var byteLen uint32
	switch enc {
	case EncUTF8:
		byteLen = codedLen
	case EncUTF16:
		byteLen = codedLen * 2
		if codedLen != 0 && byteLen/2 != codedLen {
			trapf("string byte length overflows uint32")
		}
	case EncLatin1UTF16:
		eff, isUTF16 := latin1UTF16Decode(codedLen)
		if isUTF16 {
			byteLen = eff * 2
			if eff != 0 && byteLen/2 != eff {
				trapf("string byte length overflows uint32")
			}
		} else {
			byteLen = eff
		}
	default:
		trapf("unknown string encoding")
	}
	if byteLen > maxStringByteLength {
		trapf("string byte length %d exceeds max %d", byteLen, maxStringByteLength)
	}
	if ptr%stringAlign(enc) != 0 {
		trapf("unaligned pointer %d for string encoding (align %d)", ptr, stringAlign(enc))
	}
	bytes, ok := mem.Read(ptr, byteLen)
	if !ok {
		trapf("string pointer/length out of bounds of memory: ptr=%d len=%d", ptr, byteLen)
	}
	switch enc {
	case EncUTF8:
		if msg := classifyUTF8(bytes); msg != "" {
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
