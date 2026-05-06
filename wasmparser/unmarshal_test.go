package wasmparser

import (
	"bytes"
	"testing"
)

// testItem is a simple binaryUnmarshaler for testing.
type testItem struct {
	Value uint32
}

func (t *testItem) unmarshalBinary(r *BinaryReader) error {
	v, err := r.ReadU32()
	if err != nil {
		return err
	}
	t.Value = v
	return nil
}

func TestRead(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x2A}), 0)
	item, err := unmarshalNew[*testItem](r)
	if err != nil {
		t.Fatal(err)
	}
	if item.Value != 42 {
		t.Fatalf("got %d, want 42", item.Value)
	}
}

