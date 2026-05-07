package canon

import (
	"context"
	"testing"
)

// TestMemToValRecord_OptionFieldAlignment reproduces a lifting bug where a
// record with two option<string> fields after a 1-byte field gets the second
// option's discriminant read from the wrong byte offset.
//
// Per canonical-ABI spec:
//   alignment(option(t)) = max(alignment(disc), alignment(t))
// For option<string>, alignment = max(1, 4) = 4. So in a record
// { u8, option<string>, option<string> }, the canonical layout is:
//
//   off | bytes | content
//   ----+-------+----------------------------
//    0  |   1   | u8 (status)
//    1  |   3   | padding (option<string> aligns to 4)
//    4  |   1   | message option disc
//    5  |   3   | padding
//    8  |   4   | message string ptr
//   12  |   4   | message string len
//   16  |   1   | details option disc
//   17  |   3   | padding
//   20  |   4   | details string ptr
//   24  |   4   | details string len
//
// Total record size = 28, alignment = 4.
//
// If memToValVisitor fails to align the option discriminant to its full
// (max(disc,payload)) alignment, the second option's discriminant will be
// read at offset 12 — which holds the LE low byte of the FIRST option's
// string length (= 8 for "all good"), producing
// "option: out-of-range discriminant 8".
func TestMemToValRecord_OptionFieldAlignment(t *testing.T) {
	mem := setupMem(t, 256)

	const base uint32 = 32
	const msgPtr uint32 = 96
	const detailsPtr uint32 = 128
	const msg = "all good" // 8 bytes
	const det = "extra"    // 5 bytes

	// Seed canonical layout.
	_ = mem.WriteByte(base+0, 0) // status (u8)
	// 1..3 padding (left zero, but value irrelevant)
	_ = mem.WriteByte(base+4, 1)              // message disc = Some
	_ = mem.WriteUint32Le(base+8, msgPtr)     // message string ptr
	_ = mem.WriteUint32Le(base+12, uint32(len(msg))) // message string len
	_ = mem.WriteByte(base+16, 1)             // details disc = Some
	_ = mem.WriteUint32Le(base+20, detailsPtr) // details string ptr
	_ = mem.WriteUint32Le(base+24, uint32(len(det))) // details string len
	if !mem.Write(msgPtr, []byte(msg)) {
		t.Fatal("seed msg")
	}
	if !mem.Write(detailsPtr, []byte(det)) {
		t.Fatal("seed details")
	}

	rec := testTypeRecord{fields: []RecordField{
		{Name: "status", Type: testU8Type{}},
		{Name: "message", Type: testTypeOption{inner: testTypeString{}}},
		{Name: "details", Type: testTypeOption{inner: testTypeString{}}},
	}}

	v := &memToValVisitor{}
	rec.Accept(v)

	gcc := &gocallContext{
		callee: &transferSide{Memory: mem, StringEncoding: EncUTF8},
	}
	got, err := v.out[0](context.Background(), gcc, base)
	if err != nil {
		t.Fatalf("lift failed: %v", err)
	}
	r := got.(*ValRecord)

	statusFV := r.Field("status")
	if u, ok := statusFV.(ValU8); !ok || u != 0 {
		t.Errorf("status: got %#v, want ValU8(0)", statusFV)
	}

	msgFV := r.Field("message").(*ValOption)
	if msgFV.IsNone() {
		t.Errorf("message: got None, want Some(%q)", msg)
	} else if s, ok := msgFV.Val().(ValString); !ok || string(s) != msg {
		t.Errorf("message: got %#v, want ValString(%q)", msgFV.Val(), msg)
	}

	detFV := r.Field("details").(*ValOption)
	if detFV.IsNone() {
		t.Errorf("details: got None, want Some(%q)", det)
	} else if s, ok := detFV.Val().(ValString); !ok || string(s) != det {
		t.Errorf("details: got %#v, want ValString(%q)", detFV.Val(), det)
	}
}
