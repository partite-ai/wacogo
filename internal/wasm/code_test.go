package wasm

import (
	"testing"
)

func TestLocalGet(t *testing.T) {
	var c CodeBuilder
	c.LocalGet(0)
	got := c.Bytes()
	want := []byte{0x20, 0x00}
	if !equalBytes(got, want) {
		t.Errorf("LocalGet(0): got %x, want %x", got, want)
	}

	c.Reset()
	c.LocalGet(128)
	got = c.Bytes()
	// 128 in unsigned LEB128 = 0x80 0x01
	want = []byte{0x20, 0x80, 0x01}
	if !equalBytes(got, want) {
		t.Errorf("LocalGet(128): got %x, want %x", got, want)
	}
}

func TestI32Const(t *testing.T) {
	var c CodeBuilder

	// Positive value
	c.I32Const(42)
	got := c.Bytes()
	want := []byte{0x41, 0x2a}
	if !equalBytes(got, want) {
		t.Errorf("I32Const(42): got %x, want %x", got, want)
	}

	// Negative value: -1 encodes as 0x7f in signed LEB128
	c.Reset()
	c.I32Const(-1)
	got = c.Bytes()
	want = []byte{0x41, 0x7f}
	if !equalBytes(got, want) {
		t.Errorf("I32Const(-1): got %x, want %x", got, want)
	}

	// -128 encodes as 0x80 0x7f in signed LEB128
	c.Reset()
	c.I32Const(-128)
	got = c.Bytes()
	want = []byte{0x41, 0x80, 0x7f}
	if !equalBytes(got, want) {
		t.Errorf("I32Const(-128): got %x, want %x", got, want)
	}
}

func TestI32Load(t *testing.T) {
	var c CodeBuilder
	// I32Load with align=2, offset=0
	c.I32Load(0, 2)
	got := c.Bytes()
	want := []byte{0x28, 0x02, 0x00}
	if !equalBytes(got, want) {
		t.Errorf("I32Load(0, 2): got %x, want %x", got, want)
	}

	// I32Load with align=0, offset=4
	c.Reset()
	c.I32Load(4, 0)
	got = c.Bytes()
	want = []byte{0x28, 0x00, 0x04}
	if !equalBytes(got, want) {
		t.Errorf("I32Load(4, 0): got %x, want %x", got, want)
	}
}

func TestI32Store(t *testing.T) {
	var c CodeBuilder
	// I32Store with align=2, offset=0
	c.I32Store(0, 2)
	got := c.Bytes()
	want := []byte{0x36, 0x02, 0x00}
	if !equalBytes(got, want) {
		t.Errorf("I32Store(0, 2): got %x, want %x", got, want)
	}

	// I32Store with align=0, offset=8
	c.Reset()
	c.I32Store(8, 0)
	got = c.Bytes()
	want = []byte{0x36, 0x00, 0x08}
	if !equalBytes(got, want) {
		t.Errorf("I32Store(8, 0): got %x, want %x", got, want)
	}
}

func TestCall(t *testing.T) {
	var c CodeBuilder
	c.Call(0)
	got := c.Bytes()
	want := []byte{0x10, 0x00}
	if !equalBytes(got, want) {
		t.Errorf("Call(0): got %x, want %x", got, want)
	}

	c.Reset()
	c.Call(255)
	got = c.Bytes()
	// 255 in unsigned LEB128 = 0xff 0x01
	want = []byte{0x10, 0xff, 0x01}
	if !equalBytes(got, want) {
		t.Errorf("Call(255): got %x, want %x", got, want)
	}
}

func TestBlockBrIfEnd(t *testing.T) {
	var c CodeBuilder
	// block [] ; br_if 0 ; end
	c.Block(BlockTypeEmpty)
	c.I32Const(1)
	c.BrIf(0)
	c.End()

	got := c.Bytes()
	want := []byte{
		0x02, 0x40, // block (empty)
		0x41, 0x01, // i32.const 1
		0x0d, 0x00, // br_if 0
		0x0b, // end
	}
	if !equalBytes(got, want) {
		t.Errorf("Block/BrIf/End: got %x, want %x", got, want)
	}
}

func TestIfElseEnd(t *testing.T) {
	var c CodeBuilder
	// if (empty) ; i32.const 1 ; else ; i32.const 0 ; end
	c.If(BlockTypeEmpty)
	c.I32Const(1)
	c.Else()
	c.I32Const(0)
	c.End()

	got := c.Bytes()
	want := []byte{
		0x04, 0x40, // if (empty)
		0x41, 0x01, // i32.const 1
		0x05,       // else
		0x41, 0x00, // i32.const 0
		0x0b, // end
	}
	if !equalBytes(got, want) {
		t.Errorf("If/Else/End: got %x, want %x", got, want)
	}
}

func TestMemoryCopy(t *testing.T) {
	var c CodeBuilder
	c.MemoryCopy(0, 1)
	got := c.Bytes()
	// 0xfc, uleb(10)=0x0a, uleb(0)=0x00, uleb(1)=0x01
	want := []byte{0xfc, 0x0a, 0x00, 0x01}
	if !equalBytes(got, want) {
		t.Errorf("MemoryCopy(0, 1): got %x, want %x", got, want)
	}
}

func TestCallIndirect(t *testing.T) {
	var c CodeBuilder
	c.CallIndirect(3)
	got := c.Bytes()
	// 0x11, typeIdx=3, tableIdx=0
	want := []byte{0x11, 0x03, 0x00}
	if !equalBytes(got, want) {
		t.Errorf("CallIndirect(3): got %x, want %x", got, want)
	}
}

func TestDrop(t *testing.T) {
	var c CodeBuilder
	c.Drop()
	got := c.Bytes()
	want := []byte{0x1a}
	if !equalBytes(got, want) {
		t.Errorf("Drop: got %x, want %x", got, want)
	}
}

func TestReset(t *testing.T) {
	var c CodeBuilder
	c.I32Const(1)
	c.Reset()
	if len(c.Bytes()) != 0 {
		t.Errorf("after Reset, Bytes() should be empty, got %x", c.Bytes())
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
