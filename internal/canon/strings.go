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

func decodeString(mem fakeOrRealMemory, ptr, codedLen uint32, enc StringEncoding) string {
	byteLen := codedLen * unitSize(enc)
	if codedLen != 0 && byteLen/unitSize(enc) != codedLen {
		trapf("string byte length overflows uint32")
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
		return decodeUTF16(bytes)
	}
	trapf("unknown string encoding")
	return ""
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
	case EncUTF16, EncLatin1UTF16:
		units := utf16.Encode([]rune(s))
		b := make([]byte, 2*len(units))
		for i, u := range units {
			b[2*i] = byte(u)
			b[2*i+1] = byte(u >> 8)
		}
		return b, uint32(len(units))
	}
	trapf("unknown encoding")
	return nil, 0
}

// validateUTF16Bytes is used by TransferStringStep fast path.
func validateUTF16Bytes(bytes []byte) {
	if len(bytes)%2 != 0 {
		trapf("UTF-16 byte length must be even")
	}
	_ = decodeUTF16(bytes)
}
