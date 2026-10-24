package canon

import (
	"fmt"
	"strings"
	"testing"
)

// TestTranscodedByteLen checks the discovery-walk size computation against
// the bytes encodeString actually produces, for every source/destination
// encoding pair.
func TestTranscodedByteLen(t *testing.T) {
	inputs := []string{
		"",
		"hello",
		"café ÿ",                // Latin-1 range, 2-byte UTF-8
		"Ā",                     // just past Latin-1
		"中文",                    // BMP, 3-byte UTF-8
		"\U0001F600",            // astral, surrogate pair
		"aé中\U0001F600z",        // all widths mixed
		strings.Repeat("߿", 3),  // 2-byte UTF-8 upper edge
		strings.Repeat("￿ࠀ", 2), // 3-byte UTF-8 edges
	}
	encs := []StringEncoding{EncUTF8, EncUTF16, EncLatin1UTF16}
	for _, s := range inputs {
		for _, srcEnc := range encs {
			src, srcCoded := encodeString(s, srcEnc)
			for _, dstEnc := range encs {
				t.Run(fmt.Sprintf("%q/%v->%v", s, srcEnc, dstEnc), func(t *testing.T) {
					want, _ := encodeString(s, dstEnc)
					got := transcodedByteLen(src, srcCoded, srcEnc, dstEnc)
					if got != uint32(len(want)) {
						t.Fatalf("transcodedByteLen = %d, want %d", got, len(want))
					}
				})
			}
		}
	}
}

// TestTranscodedByteLen_UTF16TaggedLatin1 covers a Latin-1+UTF-16 source
// that uses the UTF-16 sub-encoding for text that would fit in Latin-1.
func TestTranscodedByteLen_UTF16TaggedLatin1(t *testing.T) {
	s := "café"
	src, units := encodeString(s, EncUTF16)
	coded := units | latin1UTF16HighBit
	for _, dstEnc := range []StringEncoding{EncUTF8, EncUTF16} {
		want, _ := encodeString(s, dstEnc)
		if got := transcodedByteLen(src, coded, EncLatin1UTF16, dstEnc); got != uint32(len(want)) {
			t.Fatalf("%v: transcodedByteLen = %d, want %d", dstEnc, got, len(want))
		}
	}
}

func TestTranscodedByteLen_MalformedTraps(t *testing.T) {
	cases := []struct {
		name   string
		src    []byte
		coded  uint32
		srcEnc StringEncoding
		dstEnc StringEncoding
	}{
		{"bad utf-8", []byte{0xff, 0xfe}, 2, EncUTF8, EncUTF16},
		{"lone high surrogate", []byte{0x00, 0xd8}, 1, EncUTF16, EncUTF8},
		{"lone low surrogate tagged", []byte{0x00, 0xdc}, 1 | latin1UTF16HighBit, EncLatin1UTF16, EncUTF8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if _, ok := recover().(*Trap); !ok {
					t.Fatal("expected *Trap panic")
				}
			}()
			transcodedByteLen(c.src, c.coded, c.srcEnc, c.dstEnc)
		})
	}
}

func TestStringByteLen_NoWrap(t *testing.T) {
	if got := stringByteLen(0x80000001, EncUTF16); got != 0x100000002 {
		t.Fatalf("stringByteLen = %#x, want 0x100000002", got)
	}
}
