package canon

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

var transcodeInputs = []string{
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

var transcodeEncs = []StringEncoding{EncUTF8, EncUTF16, EncLatin1UTF16}

// TestTranscodedByteLen checks the discovery-walk size computation against
// the bytes encodeString actually produces, for every source/destination
// encoding pair.
func TestTranscodedByteLen(t *testing.T) {
	inputs, encs := transcodeInputs, transcodeEncs
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

// TestTranscodeInto checks the in-place transcoder against encodeString's
// bytes and coded length for every source/destination encoding pair.
func TestTranscodeInto(t *testing.T) {
	for _, s := range transcodeInputs {
		for _, srcEnc := range transcodeEncs {
			src, srcCoded := encodeString(s, srcEnc)
			for _, dstEnc := range transcodeEncs {
				t.Run(fmt.Sprintf("%q/%v->%v", s, srcEnc, dstEnc), func(t *testing.T) {
					want, wantCoded := encodeString(s, dstEnc)
					dst := make([]byte, len(want))
					if got := transcodeInto(dst, src, srcCoded, srcEnc, dstEnc); got != wantCoded {
						t.Fatalf("coded = %#x, want %#x", got, wantCoded)
					}
					if !bytes.Equal(dst, want) {
						t.Fatalf("bytes = %x, want %x", dst, want)
					}
				})
			}
		}
	}
}

// TestTranscodeInto_WrongSizeTraps checks that a destination of the wrong
// size traps instead of writing past it or leaving it short.
func TestTranscodeInto_WrongSizeTraps(t *testing.T) {
	src, coded := encodeString("aé中\U0001F600z", EncUTF8)
	n := transcodedByteLen(src, coded, EncUTF8, EncUTF16)
	for _, size := range []uint32{n - 1, n + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			defer func() {
				if _, ok := recover().(*Trap); !ok {
					t.Fatal("expected *Trap panic")
				}
			}()
			transcodeInto(make([]byte, size), src, coded, EncUTF8, EncUTF16)
		})
	}
}

// TestTranscodeInto_MalformedTraps checks the transcoder validates its
// source rather than trusting an earlier pass.
func TestTranscodeInto_MalformedTraps(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    []byte
		coded  uint32
		srcEnc StringEncoding
	}{
		{"bad utf-8", []byte{'a', 0xff}, 2, EncUTF8},
		{"truncated utf-8", []byte{'a', 0xe4, 0xb8}, 3, EncUTF8},
		{"lone high surrogate", []byte{0x00, 0xd8}, 1, EncUTF16},
		{"lone low surrogate", []byte{0x00, 0xdc}, 1, EncUTF16},
		{"odd utf-16 length", []byte{0x41}, 1 | latin1UTF16HighBit, EncLatin1UTF16},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if _, ok := recover().(*Trap); !ok {
					t.Fatal("expected *Trap panic")
				}
			}()
			transcodeInto(make([]byte, 16), c.src, c.coded, c.srcEnc, EncUTF8)
		})
	}
}

// TestLiftString_ChargesDecodedLength checks a Go string lift charges the
// UTF-8 length it allocates, not the source's byte length, and refuses
// before decoding when that exceeds the budget.
func TestLiftString_ChargesDecodedLength(t *testing.T) {
	const s = "中文中文" // 8 UTF-16 bytes, 12 UTF-8 bytes
	src, coded := encodeString(s, EncUTF16)
	mem := fakeMemory(src)

	b := LiftBudget{limit: 12}
	got, err := liftString(mem, 0, coded, EncUTF16, &b)
	if err != nil || got != s {
		t.Fatalf("liftString = %q, %v; want %q", got, err, s)
	}
	if b.used != 12 {
		t.Fatalf("charged %d, want 12", b.used)
	}

	b = LiftBudget{limit: 11}
	if _, err := liftString(mem, 0, coded, EncUTF16, &b); !errors.Is(err, ErrLiftLimitExceeded) {
		t.Fatalf("err = %v, want ErrLiftLimitExceeded", err)
	}
}

// fakeMemory is a fakeOrRealMemory over a byte slice.
type fakeMemory []byte

func (m fakeMemory) Read(off, n uint32) ([]byte, bool) {
	if uint64(off)+uint64(n) > uint64(len(m)) {
		return nil, false
	}
	return m[off : off+n], true
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
