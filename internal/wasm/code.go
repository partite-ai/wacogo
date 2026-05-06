package wasm

import "bytes"

// Block type constants used with Block, Loop, and If instructions.
const (
	BlockTypeEmpty byte = 0x40
	BlockTypeI32   byte = 0x7f
	BlockTypeI64   byte = 0x7e
	BlockTypeF32   byte = 0x7d
	BlockTypeF64   byte = 0x7c
)

// CodeBuilder emits WebAssembly instruction bytecode into an internal buffer.
// Use Bytes to retrieve the accumulated bytes and Reset to clear the buffer.
type CodeBuilder struct {
	buf bytes.Buffer
}

// Bytes returns the accumulated instruction bytes.
func (c *CodeBuilder) Bytes() []byte {
	return c.buf.Bytes()
}

// Reset clears the accumulated bytes.
func (c *CodeBuilder) Reset() {
	c.buf.Reset()
}

// writeByte writes a single opcode byte.
func (c *CodeBuilder) writeByte(b byte) {
	c.buf.WriteByte(b) //nolint:errcheck
}

// writeU32 encodes v as unsigned LEB128.
func (c *CodeBuilder) writeU32(v uint32) {
	encodeU32(&c.buf, v)
}

// writeS32 encodes v as signed LEB128.
func (c *CodeBuilder) writeS32(v int32) {
	encodeS32(&c.buf, v)
}

// writeS64 encodes v as signed LEB128.
func (c *CodeBuilder) writeS64(v int64) {
	encodeS64(&c.buf, v)
}

// --- Control flow ---

// Select emits a select instruction (0x1b). Operand stack must hold
// [value1, value2, cond]; result is value1 if cond != 0, else value2.
func (c *CodeBuilder) Select() { c.writeByte(0x1b) }

// Unreachable emits the unreachable instruction (0x00).
func (c *CodeBuilder) Unreachable() { c.writeByte(0x00) }

// Nop emits the nop instruction (0x01).
func (c *CodeBuilder) Nop() { c.writeByte(0x01) }

// Block emits a block instruction (0x02) with the given block type.
func (c *CodeBuilder) Block(blockType byte) {
	c.writeByte(0x02)
	c.writeByte(blockType)
}

// Loop emits a loop instruction (0x03) with the given block type.
func (c *CodeBuilder) Loop(blockType byte) {
	c.writeByte(0x03)
	c.writeByte(blockType)
}

// If emits an if instruction (0x04) with the given block type.
func (c *CodeBuilder) If(blockType byte) {
	c.writeByte(0x04)
	c.writeByte(blockType)
}

// Else emits the else instruction (0x05).
func (c *CodeBuilder) Else() { c.writeByte(0x05) }

// End emits the end instruction (0x0b).
func (c *CodeBuilder) End() { c.writeByte(0x0b) }

// Br emits a br instruction (0x0c) with the given label index.
func (c *CodeBuilder) Br(labelIdx uint32) {
	c.writeByte(0x0c)
	c.writeU32(labelIdx)
}

// BrIf emits a br_if instruction (0x0d) with the given label index.
func (c *CodeBuilder) BrIf(labelIdx uint32) {
	c.writeByte(0x0d)
	c.writeU32(labelIdx)
}

// Return emits a return instruction (0x0f).
func (c *CodeBuilder) Return() { c.writeByte(0x0f) }

// --- Calls ---

// Call emits a call instruction (0x10) with the given function index.
func (c *CodeBuilder) Call(funcIdx uint32) {
	c.writeByte(0x10)
	c.writeU32(funcIdx)
}

// CallIndirect emits a call_indirect instruction (0x11) with the given type
// index and an implicit table index of 0.
func (c *CodeBuilder) CallIndirect(typeIdx uint32) {
	c.writeByte(0x11)
	c.writeU32(typeIdx)
	c.writeU32(0) // table index
}

// --- Locals ---

// LocalGet emits a local.get instruction (0x20).
func (c *CodeBuilder) LocalGet(idx uint32) {
	c.writeByte(0x20)
	c.writeU32(idx)
}

// LocalSet emits a local.set instruction (0x21).
func (c *CodeBuilder) LocalSet(idx uint32) {
	c.writeByte(0x21)
	c.writeU32(idx)
}

// LocalTee emits a local.tee instruction (0x22).
func (c *CodeBuilder) LocalTee(idx uint32) {
	c.writeByte(0x22)
	c.writeU32(idx)
}

// --- Globals ---

// GlobalGet emits a global.get instruction (0x23).
func (c *CodeBuilder) GlobalGet(idx uint32) {
	c.writeByte(0x23)
	c.writeU32(idx)
}

// GlobalSet emits a global.set instruction (0x24).
func (c *CodeBuilder) GlobalSet(idx uint32) {
	c.writeByte(0x24)
	c.writeU32(idx)
}

// --- Memory loads ---
// All load/store methods take (offset, align uint32) and emit opcode, align (LEB128), offset (LEB128).

// I32Load emits an i32.load instruction (0x28).
func (c *CodeBuilder) I32Load(offset, align uint32) { c.memArg(0x28, offset, align) }

// I64Load emits an i64.load instruction (0x29).
func (c *CodeBuilder) I64Load(offset, align uint32) { c.memArg(0x29, offset, align) }

// F32Load emits an f32.load instruction (0x2a).
func (c *CodeBuilder) F32Load(offset, align uint32) { c.memArg(0x2a, offset, align) }

// F64Load emits an f64.load instruction (0x2b).
func (c *CodeBuilder) F64Load(offset, align uint32) { c.memArg(0x2b, offset, align) }

// I32Load8S emits an i32.load8_s instruction (0x2c).
func (c *CodeBuilder) I32Load8S(offset, align uint32) { c.memArg(0x2c, offset, align) }

// I32Load8U emits an i32.load8_u instruction (0x2d).
func (c *CodeBuilder) I32Load8U(offset, align uint32) { c.memArg(0x2d, offset, align) }

// I32Load16S emits an i32.load16_s instruction (0x2e).
func (c *CodeBuilder) I32Load16S(offset, align uint32) { c.memArg(0x2e, offset, align) }

// I32Load16U emits an i32.load16_u instruction (0x2f).
func (c *CodeBuilder) I32Load16U(offset, align uint32) { c.memArg(0x2f, offset, align) }

// --- Memory stores ---

// I32Store emits an i32.store instruction (0x36).
func (c *CodeBuilder) I32Store(offset, align uint32) { c.memArg(0x36, offset, align) }

// I64Store emits an i64.store instruction (0x37).
func (c *CodeBuilder) I64Store(offset, align uint32) { c.memArg(0x37, offset, align) }

// F32Store emits an f32.store instruction (0x38).
func (c *CodeBuilder) F32Store(offset, align uint32) { c.memArg(0x38, offset, align) }

// F64Store emits an f64.store instruction (0x39).
func (c *CodeBuilder) F64Store(offset, align uint32) { c.memArg(0x39, offset, align) }

// I32Store8 emits an i32.store8 instruction (0x3a).
func (c *CodeBuilder) I32Store8(offset, align uint32) { c.memArg(0x3a, offset, align) }

// I32Store16 emits an i32.store16 instruction (0x3b).
func (c *CodeBuilder) I32Store16(offset, align uint32) { c.memArg(0x3b, offset, align) }

// memArg emits a memory instruction with alignment and offset operands.
func (c *CodeBuilder) memArg(opcode byte, offset, align uint32) {
	c.writeByte(opcode)
	c.writeU32(align)
	c.writeU32(offset)
}

// --- Constants ---

// I32Const emits an i32.const instruction (0x41) with a signed LEB128 value.
func (c *CodeBuilder) I32Const(v int32) {
	c.writeByte(0x41)
	c.writeS32(v)
}

// I64Const emits an i64.const instruction (0x42) with a signed LEB128 value.
func (c *CodeBuilder) I64Const(v int64) {
	c.writeByte(0x42)
	c.writeS64(v)
}

// --- Arithmetic / comparison ---

// I32Eqz emits an i32.eqz instruction (0x45).
func (c *CodeBuilder) I32Eqz() { c.writeByte(0x45) }

// I32Eq emits an i32.eq instruction (0x46).
func (c *CodeBuilder) I32Eq() { c.writeByte(0x46) }

// I32Ne emits an i32.ne instruction (0x47).
func (c *CodeBuilder) I32Ne() { c.writeByte(0x47) }

// I32LtU emits an i32.lt_u instruction (0x49).
func (c *CodeBuilder) I32LtU() { c.writeByte(0x49) }

// I32GtU emits an i32.gt_u instruction (0x4b).
func (c *CodeBuilder) I32GtU() { c.writeByte(0x4b) }

// I32LeU emits an i32.le_u instruction (0x4d).
func (c *CodeBuilder) I32LeU() { c.writeByte(0x4d) }

// I32GeU emits an i32.ge_u instruction (0x4f).
func (c *CodeBuilder) I32GeU() { c.writeByte(0x4f) }

// I32Add emits an i32.add instruction (0x6a).
func (c *CodeBuilder) I32Add() { c.writeByte(0x6a) }

// I32Sub emits an i32.sub instruction (0x6b).
func (c *CodeBuilder) I32Sub() { c.writeByte(0x6b) }

// I32Mul emits an i32.mul instruction (0x6c).
func (c *CodeBuilder) I32Mul() { c.writeByte(0x6c) }

// I32And emits an i32.and instruction (0x71).
func (c *CodeBuilder) I32And() { c.writeByte(0x71) }

// I32Or emits an i32.or instruction (0x72).
func (c *CodeBuilder) I32Or() { c.writeByte(0x72) }

// I32Shl emits an i32.shl instruction (0x74).
func (c *CodeBuilder) I32Shl() { c.writeByte(0x74) }

// I32ShrS emits an i32.shr_s instruction (0x75).
func (c *CodeBuilder) I32ShrS() { c.writeByte(0x75) }

// I32ShrU emits an i32.shr_u instruction (0x76).
func (c *CodeBuilder) I32ShrU() { c.writeByte(0x76) }

// I32WrapI64 emits an i32.wrap_i64 instruction (0xa7).
func (c *CodeBuilder) I32WrapI64() { c.writeByte(0xa7) }

// I64ExtendI32S emits an i64.extend_i32_s instruction (0xac).
func (c *CodeBuilder) I64ExtendI32S() { c.writeByte(0xac) }

// I64ExtendI32U emits an i64.extend_i32_u instruction (0xad).
func (c *CodeBuilder) I64ExtendI32U() { c.writeByte(0xad) }

// --- Memory ops ---

// MemorySize emits a memory.size instruction (0x3f) for the given memory index.
func (c *CodeBuilder) MemorySize(memIdx uint32) {
	c.writeByte(0x3f)
	c.writeU32(memIdx)
}

// MemoryGrow emits a memory.grow instruction (0x40) for the given memory index.
func (c *CodeBuilder) MemoryGrow(memIdx uint32) {
	c.writeByte(0x40)
	c.writeU32(memIdx)
}

// MemoryCopy emits a memory.copy instruction (0xfc 10) with destination and
// source memory indices.
func (c *CodeBuilder) MemoryCopy(dstMem, srcMem uint32) {
	c.writeByte(0xfc)
	c.writeU32(10) // sub-opcode for memory.copy
	c.writeU32(dstMem)
	c.writeU32(srcMem)
}

// --- Sign extension (sign extension ops proposal, now standard) ---

// I32Extend8S emits an i32.extend8_s instruction (0xc0).
func (c *CodeBuilder) I32Extend8S() { c.writeByte(0xc0) }

// I32Extend16S emits an i32.extend16_s instruction (0xc1).
func (c *CodeBuilder) I32Extend16S() { c.writeByte(0xc1) }

// I64Extend8S emits an i64.extend8_s instruction (0xc2).
func (c *CodeBuilder) I64Extend8S() { c.writeByte(0xc2) }

// I64Extend16S emits an i64.extend16_s instruction (0xc3).
func (c *CodeBuilder) I64Extend16S() { c.writeByte(0xc3) }

// I64Extend32S emits an i64.extend32_s instruction (0xc4).
func (c *CodeBuilder) I64Extend32S() { c.writeByte(0xc4) }

// --- Other ---

// Drop emits a drop instruction (0x1a).
func (c *CodeBuilder) Drop() { c.writeByte(0x1a) }
