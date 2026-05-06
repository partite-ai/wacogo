package wasm

import "bytes"

// Core wasm value types.
const (
	ValI32 byte = 0x7f
	ValI64 byte = 0x7e
	ValF32 byte = 0x7d
	ValF64 byte = 0x7c
)

// FuncSig represents a core wasm function signature.
type FuncSig struct {
	Params  []byte // each byte is a ValType constant
	Results []byte
}

// LocalEntry declares local variables in a function body.
type LocalEntry struct {
	Count   uint32
	ValType byte
}

// NewLocalEntry constructs a LocalEntry.
func NewLocalEntry(count uint32, valType byte) LocalEntry {
	return LocalEntry{Count: count, ValType: valType}
}

// importFunc records an imported function.
type importFunc struct {
	module  string
	name    string
	typeIdx uint32
}

// importMemory records an imported memory.
type importMemory struct {
	module   string
	name     string
	minPages uint32
	maxPages uint32 // 0 means no max
}

// importTable records an imported table.
type importTable struct {
	module  string
	name    string
	min     uint32
	max     uint32 // 0 means no max
	refType byte   // 0x70 = funcref, 0x6f = externref
}

// importGlobal records an imported global.
type importGlobal struct {
	module  string
	name    string
	valType byte
	mutable bool
}

// funcDef records a defined function.
type funcDef struct {
	typeIdx uint32
	locals  []LocalEntry
	body    []byte
}

// memDef records a defined memory.
type memDef struct {
	minPages uint32
	maxPages uint32 // 0 means no max
}

// exportEntry records an export.
type exportEntry struct {
	name    string
	kind    byte   // 0x00 = func, 0x02 = memory
	idx     uint32
}

// globalDef records a defined global.
type globalDef struct {
	valType byte
	mutable bool
	initI32 int32
}

// dataSegment records an active data segment.
type dataSegment struct {
	offset uint32
	data   []byte
}

// ModuleBuilder constructs a valid core wasm module binary from parts.
//
// Indices follow the wasm spec ordering: imported functions first, then defined
// functions. Likewise for memories, tables, and globals.
type ModuleBuilder struct {
	types       []FuncSig // deduplicated type table
	importFns   []importFunc
	importMems  []importMemory
	importTbls  []importTable
	importGlobs []importGlobal
	funcs       []funcDef
	mems        []memDef
	globals     []globalDef
	exports     []exportEntry
	dataSegs    []dataSegment
}

// internType returns the index for sig, adding it to the type table if needed.
func (b *ModuleBuilder) internType(sig FuncSig) uint32 {
	for i, t := range b.types {
		if sigsEqual(t, sig) {
			return uint32(i)
		}
	}
	idx := uint32(len(b.types))
	b.types = append(b.types, sig)
	return idx
}

func sigsEqual(a, b FuncSig) bool {
	if len(a.Params) != len(b.Params) || len(a.Results) != len(b.Results) {
		return false
	}
	for i := range a.Params {
		if a.Params[i] != b.Params[i] {
			return false
		}
	}
	for i := range a.Results {
		if a.Results[i] != b.Results[i] {
			return false
		}
	}
	return true
}

// AddImportFunc adds a function import and returns its function index.
func (b *ModuleBuilder) AddImportFunc(module, name string, sig FuncSig) uint32 {
	typeIdx := b.internType(sig)
	idx := uint32(len(b.importFns))
	b.importFns = append(b.importFns, importFunc{module: module, name: name, typeIdx: typeIdx})
	return idx
}

// AddImportMemory adds a memory import and returns its memory index.
// maxPages=0 means unbounded.
func (b *ModuleBuilder) AddImportMemory(module, name string, minPages, maxPages uint32) uint32 {
	idx := uint32(len(b.importMems))
	b.importMems = append(b.importMems, importMemory{module: module, name: name, minPages: minPages, maxPages: maxPages})
	return idx
}

// AddImportTable adds a table import and returns its table index.
// refType is 0x70 for funcref, 0x6f for externref. max=0 means unbounded.
func (b *ModuleBuilder) AddImportTable(module, name string, min, max uint32, refType byte) uint32 {
	idx := uint32(len(b.importTbls))
	b.importTbls = append(b.importTbls, importTable{module: module, name: name, min: min, max: max, refType: refType})
	return idx
}

// AddImportGlobal adds a global import and returns its global index.
func (b *ModuleBuilder) AddImportGlobal(module, name string, valType byte, mutable bool) uint32 {
	idx := uint32(len(b.importGlobs))
	b.importGlobs = append(b.importGlobs, importGlobal{module: module, name: name, valType: valType, mutable: mutable})
	return idx
}

// AddFunc adds a defined function and returns its function index (imports first).
func (b *ModuleBuilder) AddFunc(sig FuncSig, locals []LocalEntry, body []byte) uint32 {
	typeIdx := b.internType(sig)
	funcIdx := uint32(len(b.importFns)) + uint32(len(b.funcs))
	b.funcs = append(b.funcs, funcDef{typeIdx: typeIdx, locals: locals, body: body})
	return funcIdx
}

// AddMemory adds a defined memory and returns its memory index (imports first).
// maxPages=0 means no maximum.
func (b *ModuleBuilder) AddMemory(minPages, maxPages uint32) uint32 {
	memIdx := uint32(len(b.importMems)) + uint32(len(b.mems))
	b.mems = append(b.mems, memDef{minPages: minPages, maxPages: maxPages})
	return memIdx
}

// AddGlobal adds a defined global with an i32.const init expression and returns its global index.
func (b *ModuleBuilder) AddGlobal(valType byte, mutable bool, initVal int32) uint32 {
	idx := uint32(len(b.globals))
	b.globals = append(b.globals, globalDef{valType: valType, mutable: mutable, initI32: initVal})
	return idx
}

// AddExportGlobal adds a global export.
func (b *ModuleBuilder) AddExportGlobal(name string, globalIdx uint32) {
	b.exports = append(b.exports, exportEntry{name: name, kind: 0x03, idx: globalIdx})
}

// AddExportFunc adds a function export.
func (b *ModuleBuilder) AddExportFunc(name string, funcIdx uint32) {
	b.exports = append(b.exports, exportEntry{name: name, kind: 0x00, idx: funcIdx})
}

// AddExportMemory adds a memory export.
func (b *ModuleBuilder) AddExportMemory(name string, memIdx uint32) {
	b.exports = append(b.exports, exportEntry{name: name, kind: 0x02, idx: memIdx})
}

// AddExportTable adds a table export.
func (b *ModuleBuilder) AddExportTable(name string, tableIdx uint32) {
	b.exports = append(b.exports, exportEntry{name: name, kind: 0x01, idx: tableIdx})
}

// AddDataSegment adds an active data segment at memory index 0 at the given byte offset.
func (b *ModuleBuilder) AddDataSegment(offset uint32, data []byte) {
	b.dataSegs = append(b.dataSegs, dataSegment{offset: offset, data: data})
}

// Encode serialises the module to a wasm binary.
func (b *ModuleBuilder) Encode() []byte {
	var out bytes.Buffer

	// Magic + version
	out.Write([]byte{0x00, 0x61, 0x73, 0x6d}) // \0asm
	out.Write([]byte{0x01, 0x00, 0x00, 0x00}) // version 1

	// Section 1: Type
	if len(b.types) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.types)))
		for _, sig := range b.types {
			sec.WriteByte(0x60) // func type
			encodeVec(&sec, uint32(len(sig.Params)))
			sec.Write(sig.Params)
			encodeVec(&sec, uint32(len(sig.Results)))
			sec.Write(sig.Results)
		}
		writeSection(&out, 1, sec.Bytes())
	}

	// Section 2: Import
	totalImports := len(b.importFns) + len(b.importTbls) + len(b.importMems) + len(b.importGlobs)
	if totalImports > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(totalImports))
		for _, imp := range b.importFns {
			encodeName(&sec, imp.module)
			encodeName(&sec, imp.name)
			sec.WriteByte(0x00) // kind: func
			encodeU32(&sec, imp.typeIdx)
		}
		for _, imp := range b.importTbls {
			encodeName(&sec, imp.module)
			encodeName(&sec, imp.name)
			sec.WriteByte(0x01) // kind: table
			sec.WriteByte(imp.refType)
			encodeLimits(&sec, imp.min, imp.max)
		}
		for _, imp := range b.importMems {
			encodeName(&sec, imp.module)
			encodeName(&sec, imp.name)
			sec.WriteByte(0x02) // kind: memory
			encodeLimits(&sec, imp.minPages, imp.maxPages)
		}
		for _, imp := range b.importGlobs {
			encodeName(&sec, imp.module)
			encodeName(&sec, imp.name)
			sec.WriteByte(0x03) // kind: global
			sec.WriteByte(imp.valType)
			if imp.mutable {
				sec.WriteByte(0x01)
			} else {
				sec.WriteByte(0x00)
			}
		}
		writeSection(&out, 2, sec.Bytes())
	}

	// Section 3: Function (type indices for defined functions)
	if len(b.funcs) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.funcs)))
		for _, fn := range b.funcs {
			encodeU32(&sec, fn.typeIdx)
		}
		writeSection(&out, 3, sec.Bytes())
	}

	// Section 5: Memory (defined memories)
	if len(b.mems) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.mems)))
		for _, m := range b.mems {
			encodeLimits(&sec, m.minPages, m.maxPages)
		}
		writeSection(&out, 5, sec.Bytes())
	}

	// Section 6: Global
	if len(b.globals) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.globals)))
		for _, g := range b.globals {
			sec.WriteByte(g.valType)
			if g.mutable {
				sec.WriteByte(0x01)
			} else {
				sec.WriteByte(0x00)
			}
			// Init expression: i32.const <value>, end
			sec.WriteByte(0x41) // i32.const
			encodeS32(&sec, g.initI32)
			sec.WriteByte(0x0b) // end
		}
		writeSection(&out, 6, sec.Bytes())
	}

	// Section 7: Export
	if len(b.exports) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.exports)))
		for _, exp := range b.exports {
			encodeName(&sec, exp.name)
			sec.WriteByte(exp.kind)
			encodeU32(&sec, exp.idx)
		}
		writeSection(&out, 7, sec.Bytes())
	}

	// Section 12: Data count (must come before code section when data section is present)
	if len(b.dataSegs) > 0 {
		var sec bytes.Buffer
		encodeU32(&sec, uint32(len(b.dataSegs)))
		writeSection(&out, 12, sec.Bytes())
	}

	// Section 10: Code
	if len(b.funcs) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.funcs)))
		for _, fn := range b.funcs {
			var body bytes.Buffer
			// locals
			encodeVec(&body, uint32(len(fn.locals)))
			for _, loc := range fn.locals {
				encodeU32(&body, loc.Count)
				body.WriteByte(loc.ValType)
			}
			// instructions (already include the final `end` byte)
			body.Write(fn.body)
			// encode body with its length prefix
			encodeU32(&sec, uint32(body.Len()))
			sec.Write(body.Bytes())
		}
		writeSection(&out, 10, sec.Bytes())
	}

	// Section 11: Data
	if len(b.dataSegs) > 0 {
		var sec bytes.Buffer
		encodeVec(&sec, uint32(len(b.dataSegs)))
		for _, ds := range b.dataSegs {
			sec.WriteByte(0x00)           // active segment, memory 0
			sec.WriteByte(0x41)           // i32.const
			encodeS32(&sec, int32(ds.offset))
			sec.WriteByte(0x0b)           // end
			encodeVec(&sec, uint32(len(ds.data)))
			sec.Write(ds.data)
		}
		writeSection(&out, 11, sec.Bytes())
	}

	return out.Bytes()
}

// encodeLimits writes a wasm limits type. maxPages=0 means unbounded (flags=0x00).
func encodeLimits(w *bytes.Buffer, minPages, maxPages uint32) {
	if maxPages == 0 {
		w.WriteByte(0x00) // no max
		encodeU32(w, minPages)
	} else {
		w.WriteByte(0x01) // has max
		encodeU32(w, minPages)
		encodeU32(w, maxPages)
	}
}

// writeSection writes a section with the given id and data payload to out.
func writeSection(out *bytes.Buffer, id byte, data []byte) {
	out.WriteByte(id)
	encodeU32(out, uint32(len(data)))
	out.Write(data)
}
