package wasmparser

import (
	"bytes"
	"fmt"
)

// parseInnerCoreModule consumes the sub-parser's binary reader to parse an
// inline core wasm module and extract its type information (imports and exports).
func (a *TypeArena) parseInnerCoreModule(sub *Parser) (CoreModuleTypeDesc, error) {
	modType := CoreModuleTypeDesc{
		Imports: make(map[importKey]CoreEntityType),
		Exports: make(map[string]CoreEntityType),
	}

	r := sub.reader

	// Skip 8-byte module header (magic + version)
	_, err := r.ReadBytes(8)
	if err != nil {
		return modType, fmt.Errorf("reading module header: %w", err)
	}

	// Core wasm section IDs
	const (
		coreCustomSection = 0
		coreTypeSection   = 1
		coreImportSection = 2
		coreFuncSection   = 3
		coreTableSection  = 4
		coreMemSection    = 5
		coreGlobalSection = 6
		coreExportSection = 7
	)

	// Index spaces built from the module
	var funcTypes []CoreFuncTypeDesc
	var importedFuncIDs []CoreFuncTypeID // arena ID per imported func, indexed by func-index space
	var funcSigs []uint32                // typeindex for each defined func
	var tables []CoreTableType
	var memories []CoreMemoryType
	var globals []CoreGlobalType

	var importedFuncs, importedTables, importedMems, importedGlobals int

	for !r.IsEOF() {
		sectionID, err := r.ReadByte()
		if err != nil {
			return modType, err
		}
		sectionLen, err := r.ReadU32()
		if err != nil {
			return modType, err
		}
		sectionStart := r.Offset()
		safeLen, err := checkSectionLength(sectionStart, sectionLen, "inner core module section")
		if err != nil {
			return modType, err
		}
		sectionData, err := r.ReadBytes(safeLen)
		if err != nil {
			return modType, err
		}
		_ = sectionStart

		sr := newBinaryReaderAt(bytes.NewReader(sectionData), sectionStart)

		switch sectionID {
		case coreTypeSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxTypes, "inner module type"); err != nil {
				return modType, err
			}
			for range count {
				ft, err := readInlineFuncType(sr)
				if err != nil {
					return modType, err
				}
				funcTypes = append(funcTypes, ft)
			}

		case coreImportSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxImports, "inner module import"); err != nil {
				return modType, err
			}
			for range count {
				module, err := sr.ReadString()
				if err != nil {
					return modType, err
				}
				name, err := sr.ReadString()
				if err != nil {
					return modType, err
				}
				kind, err := sr.ReadByte()
				if err != nil {
					return modType, err
				}
				var et CoreEntityType
				switch kind {
				case 0x00: // func
					typeIdx, err := sr.ReadU32()
					if err != nil {
						return modType, err
					}
					if int(typeIdx) >= len(funcTypes) {
						return modType, fmt.Errorf("import func type index out of bounds")
					}
					fid := a.pushCoreFuncType(funcTypes[typeIdx])
					et = CoreEntityType{Kind: CoreEntityFunc, Func: fid}
					importedFuncIDs = append(importedFuncIDs, fid)
					importedFuncs++
				case 0x01: // table
					tbl, err := readInlineTableType(sr)
					if err != nil {
						return modType, err
					}
					et = CoreEntityType{Kind: CoreEntityTable, Table: tbl}
					tables = append(tables, tbl)
					importedTables++
				case 0x02: // memory
					mem, err := readInlineMemoryType(sr)
					if err != nil {
						return modType, err
					}
					et = CoreEntityType{Kind: CoreEntityMemory, Memory: mem}
					memories = append(memories, mem)
					importedMems++
				case 0x03: // global
					g, err := readInlineGlobalType(sr)
					if err != nil {
						return modType, err
					}
					et = CoreEntityType{Kind: CoreEntityGlobal, Global: g}
					globals = append(globals, g)
					importedGlobals++
				default:
					return modType, fmt.Errorf("unknown import kind: 0x%02x", kind)
				}
				key := importKey{Module: module, Name: name}
				if _, exists := modType.Imports[key]; exists {
					return modType, fmt.Errorf("duplicate import name `%s:%s`", module, name)
				}
				modType.Imports[key] = et
				modType.ImportOrder = append(modType.ImportOrder, key)
			}

		case coreFuncSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxFunctions, "inner module func"); err != nil {
				return modType, err
			}
			for range count {
				typeIdx, err := sr.ReadU32()
				if err != nil {
					return modType, err
				}
				funcSigs = append(funcSigs, typeIdx)
			}

		case coreTableSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxTables, "inner module table"); err != nil {
				return modType, err
			}
			for range count {
				tbl, err := readInlineTableType(sr)
				if err != nil {
					return modType, err
				}
				tables = append(tables, tbl)
			}

		case coreMemSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxMemories, "inner module memory"); err != nil {
				return modType, err
			}
			for range count {
				mem, err := readInlineMemoryType(sr)
				if err != nil {
					return modType, err
				}
				memories = append(memories, mem)
			}

		case coreGlobalSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxGlobals, "inner module global"); err != nil {
				return modType, err
			}
			for range count {
				g, err := readInlineGlobalType(sr)
				if err != nil {
					return modType, err
				}
				globals = append(globals, g)
				if err := skipInitExpr(sr); err != nil {
					return modType, fmt.Errorf("global init expr: %w", err)
				}
			}

		case coreExportSection:
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxExports, "inner module export"); err != nil {
				return modType, err
			}
			for range count {
				name, err := sr.ReadString()
				if err != nil {
					return modType, err
				}
				kind, err := sr.ReadByte()
				if err != nil {
					return modType, err
				}
				idx, err := sr.ReadU32()
				if err != nil {
					return modType, err
				}
				var et CoreEntityType
				switch kind {
				case 0x00: // func
					totalFuncs := importedFuncs + len(funcSigs)
					if int(idx) >= totalFuncs {
						return modType, fmt.Errorf("export func index out of bounds")
					}
					if int(idx) < importedFuncs {
						et = CoreEntityType{Kind: CoreEntityFunc, Func: importedFuncIDs[idx]}
					} else {
						typeIdx := funcSigs[int(idx)-importedFuncs]
						if int(typeIdx) >= len(funcTypes) {
							return modType, fmt.Errorf("func type index out of bounds")
						}
						fid := a.pushCoreFuncType(funcTypes[typeIdx])
						et = CoreEntityType{Kind: CoreEntityFunc, Func: fid}
					}
				case 0x01: // table
					if int(idx) >= len(tables) {
						return modType, fmt.Errorf("export table index out of bounds")
					}
					et = CoreEntityType{Kind: CoreEntityTable, Table: tables[idx]}
				case 0x02: // memory
					if int(idx) >= len(memories) {
						return modType, fmt.Errorf("export memory index out of bounds")
					}
					et = CoreEntityType{Kind: CoreEntityMemory, Memory: memories[idx]}
				case 0x03: // global
					if int(idx) >= len(globals) {
						return modType, fmt.Errorf("export global index out of bounds")
					}
					et = CoreEntityType{Kind: CoreEntityGlobal, Global: globals[idx]}
				default:
					return modType, fmt.Errorf("unknown export kind: 0x%02x", kind)
				}
				modType.Exports[name] = et
			}

		case 10: // code section
			count, err := sr.ReadU32()
			if err != nil {
				return modType, err
			}
			if err := checkCount(sr.Offset(), count, MaxFunctions, "inner module code"); err != nil {
				return modType, err
			}
			for i := range count {
				bodySize, err := sr.ReadU32()
				if err != nil {
					return modType, err
				}
				if uint64(bodySize) > MaxFunctionBodySize {
					return modType, errfAt(sr.Offset(), "function body size %d exceeds maximum %d", bodySize, MaxFunctionBodySize)
				}
				bodyData, err := sr.ReadBytes(int(bodySize))
				if err != nil {
					return modType, err
				}
				if int(i) >= len(funcSigs) {
					continue
				}
				typeIdx := funcSigs[i]
				if int(typeIdx) >= len(funcTypes) {
					return modType, fmt.Errorf("func type index out of bounds in code section")
				}
				ft := funcTypes[typeIdx]
				if err := validateFuncBody(ft, bodyData, int(sr.Offset())-len(bodyData)); err != nil {
					return modType, err
				}
			}

		default:
			// Skip other sections (data, element, custom, etc.)
		}
	}

	// Mark sub-parser as consumed so ParseAll doesn't re-iterate
	sub.state = stateEnd

	return modType, nil
}

// validateFuncBody performs basic type-checking of a core wasm function body.
// This is a simplified validator that checks operand stack consistency for
// common instructions (enough to catch basic type mismatches).
func validateFuncBody(ft CoreFuncTypeDesc, body []byte, baseOffset int) error {
	br := newBinaryReaderAt(bytes.NewReader(body), uint64(baseOffset))

	// Parse locals
	localCount, err := br.ReadU32()
	if err != nil {
		return err
	}
	if err := checkCount(br.Offset(), localCount, MaxFunctions, "function locals groups"); err != nil {
		return err
	}
	var localTypes []CoreValType
	localTypes = append(localTypes, ft.Params...)
	// Track total locals against a flat cap so an attacker can't drive 4 GB
	// of slice growth via many small groups either.
	var totalLocals uint64
	for range localCount {
		count, err := br.ReadU32()
		if err != nil {
			return err
		}
		vt, err := br.ReadByte()
		if err != nil {
			return err
		}
		totalLocals += uint64(count)
		if totalLocals > MaxFunctions {
			return errfAt(br.Offset(), "function local count %d exceeds maximum %d", totalLocals, uint64(MaxFunctions))
		}
		cvt := CoreValType(vt)
		for range count {
			localTypes = append(localTypes, cvt)
		}
	}

	// Type-check instructions with a value stack
	var stack []CoreValType

	pop := func(expected CoreValType) error {
		if len(stack) == 0 {
			return fmt.Errorf("type mismatch")
		}
		actual := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if actual != expected {
			return fmt.Errorf("type mismatch")
		}
		return nil
	}
	push := func(t CoreValType) {
		stack = append(stack, t)
	}

	for !br.IsEOF() {
		opcode, err := br.ReadByte()
		if err != nil {
			return err
		}

		switch opcode {
		case 0x0b: // end
			if len(ft.Results) != len(stack) {
				return fmt.Errorf("type mismatch")
			}
			for i := range ft.Results {
				if ft.Results[i] != stack[i] {
					return fmt.Errorf("type mismatch")
				}
			}
			return nil

		case 0x01: // nop
			// no-op

		case 0x00: // unreachable
			return nil

		case 0x20: // local.get
			idx, err := br.ReadU32()
			if err != nil {
				return err
			}
			if int(idx) >= len(localTypes) {
				return fmt.Errorf("unknown local")
			}
			push(localTypes[idx])

		case 0x41: // i32.const
			if _, err := readLEB128Signed(br); err != nil {
				return err
			}
			push(CoreValTypeI32)

		case 0x42: // i64.const
			if _, err := readLEB128Signed(br); err != nil {
				return err
			}
			push(CoreValTypeI64)

		case 0x43: // f32.const
			if _, err := br.ReadBytes(4); err != nil {
				return err
			}
			push(CoreValTypeF32)

		case 0x44: // f64.const
			if _, err := br.ReadBytes(8); err != nil {
				return err
			}
			push(CoreValTypeF64)

		// i32 binary ops (add, sub, mul, div_s, div_u, rem_s, rem_u, and, or, xor, shl, shr_s, shr_u, rotl, rotr)
		case 0x6a, 0x6b, 0x6c, 0x6d, 0x6e, 0x6f, 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77:
			if err := pop(CoreValTypeI32); err != nil {
				return err
			}
			if err := pop(CoreValTypeI32); err != nil {
				return err
			}
			push(CoreValTypeI32)

		default:
			// Unrecognized instruction — skip validation of this body.
			return nil
		}
	}

	return nil
}

func readLEB128Signed(br *BinaryReader) (int64, error) {
	var result int64
	var shift uint
	for {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= int64(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			if shift < 64 && (b&0x40) != 0 {
				result |= -(1 << shift)
			}
			return result, nil
		}
	}
}

// readInlineFuncType reads a core wasm function type for inline module parsing.
func readInlineFuncType(r *BinaryReader) (CoreFuncTypeDesc, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return CoreFuncTypeDesc{}, err
	}
	if tag != 0x60 {
		return CoreFuncTypeDesc{}, fmt.Errorf("expected func type tag 0x60, got 0x%02x", tag)
	}
	paramCount, err := r.ReadU32()
	if err != nil {
		return CoreFuncTypeDesc{}, err
	}
	if err := checkCount(r.Offset(), paramCount, MaxFunctionParams, "function param"); err != nil {
		return CoreFuncTypeDesc{}, err
	}
	params := make([]CoreValType, paramCount)
	for i := range params {
		b, err := r.ReadByte()
		if err != nil {
			return CoreFuncTypeDesc{}, err
		}
		cvt, err := coreValTypeFromByte(b)
		if err != nil {
			return CoreFuncTypeDesc{}, err
		}
		params[i] = cvt
	}
	resultCount, err := r.ReadU32()
	if err != nil {
		return CoreFuncTypeDesc{}, err
	}
	if err := checkCount(r.Offset(), resultCount, MaxFunctionResults, "function result"); err != nil {
		return CoreFuncTypeDesc{}, err
	}
	results := make([]CoreValType, resultCount)
	for i := range results {
		b, err := r.ReadByte()
		if err != nil {
			return CoreFuncTypeDesc{}, err
		}
		cvt, err := coreValTypeFromByte(b)
		if err != nil {
			return CoreFuncTypeDesc{}, err
		}
		results[i] = cvt
	}
	return CoreFuncTypeDesc{Params: params, Results: results}, nil
}

// readInlineTableType reads a core wasm table type for inline module parsing.
func readInlineTableType(r *BinaryReader) (CoreTableType, error) {
	elemType, err := r.ReadByte()
	if err != nil {
		return CoreTableType{}, err
	}
	cvt, err := coreValTypeFromByte(elemType)
	if err != nil {
		return CoreTableType{}, err
	}
	min, max, err := readInlineLimits(r)
	if err != nil {
		return CoreTableType{}, err
	}
	return CoreTableType{
		ElemType: cvt,
		Min:      uint32(min),
		Max: func() Optional[uint32] {
			if max.Valid {
				return Some(uint32(max.Value))
			}
			return Optional[uint32]{}
		}(),
	}, nil
}

// readInlineMemoryType reads a core wasm memory type for inline module parsing.
func readInlineMemoryType(r *BinaryReader) (CoreMemoryType, error) {
	flags, err := r.ReadByte()
	if err != nil {
		return CoreMemoryType{}, err
	}
	hasMax := flags&0x01 != 0
	shared := flags&0x02 != 0
	mem64 := flags&0x04 != 0

	min, err := r.ReadU32()
	if err != nil {
		return CoreMemoryType{}, err
	}
	var max Optional[uint64]
	if hasMax {
		m, err := r.ReadU32()
		if err != nil {
			return CoreMemoryType{}, err
		}
		max = Some(uint64(m))
	}
	return CoreMemoryType{
		Min:    uint64(min),
		Max:    max,
		Shared: shared,
		Mem64:  mem64,
	}, nil
}

// readInlineGlobalType reads a core wasm global type for inline module parsing.
func readInlineGlobalType(r *BinaryReader) (CoreGlobalType, error) {
	valType, err := r.ReadByte()
	if err != nil {
		return CoreGlobalType{}, err
	}
	cvt, err := coreValTypeFromByte(valType)
	if err != nil {
		return CoreGlobalType{}, err
	}
	mutByte, err := r.ReadByte()
	if err != nil {
		return CoreGlobalType{}, err
	}
	return CoreGlobalType{
		ValType: cvt,
		Mutable: mutByte == 1,
	}, nil
}

// readInlineLimits reads a wasm limits (flag byte + min + optional max).
func readInlineLimits(r *BinaryReader) (min uint64, max Optional[uint64], err error) {
	flags, err := r.ReadByte()
	if err != nil {
		return 0, Optional[uint64]{}, err
	}
	hasMax := flags&0x01 != 0
	minVal, err := r.ReadU32()
	if err != nil {
		return 0, Optional[uint64]{}, err
	}
	if hasMax {
		maxVal, err := r.ReadU32()
		if err != nil {
			return 0, Optional[uint64]{}, err
		}
		return uint64(minVal), Some(uint64(maxVal)), nil
	}
	return uint64(minVal), Optional[uint64]{}, nil
}

// skipInitExpr consumes a constant init expression (sequence of constant
// instructions terminated by 0x0B end). Only the instruction forms that
// appear in core wasm init expressions are supported — enough to walk past
// a global's initializer without interpreting it.
func skipInitExpr(r *BinaryReader) error {
	for {
		op, err := r.ReadByte()
		if err != nil {
			return err
		}
		switch op {
		case 0x0B: // end
			return nil
		case 0x41: // i32.const s32
			if _, err := r.ReadS32(); err != nil {
				return err
			}
		case 0x42: // i64.const s64
			if _, err := r.ReadS64(); err != nil {
				return err
			}
		case 0x43: // f32.const f32 (4 bytes)
			if _, err := r.ReadBytes(4); err != nil {
				return err
			}
		case 0x44: // f64.const f64 (8 bytes)
			if _, err := r.ReadBytes(8); err != nil {
				return err
			}
		case 0x23: // global.get u32
			if _, err := r.ReadU32(); err != nil {
				return err
			}
		case 0xD0: // ref.null heaptype
			if _, err := r.ReadByte(); err != nil {
				return err
			}
		case 0xD2: // ref.func u32
			if _, err := r.ReadU32(); err != nil {
				return err
			}
		case 0x6A, 0x6B, 0x6C, // i32.add, i32.sub, i32.mul (extended-const)
			0x7C, 0x7D, 0x7E: // i64.add, i64.sub, i64.mul (extended-const)
			// no immediates
		default:
			return fmt.Errorf("unsupported init expr opcode: 0x%02x", op)
		}
	}
}

func coreValTypeFromByte(b byte) (CoreValType, error) {
	switch b {
	case 0x7f:
		return CoreValTypeI32, nil
	case 0x7e:
		return CoreValTypeI64, nil
	case 0x7d:
		return CoreValTypeF32, nil
	case 0x7c:
		return CoreValTypeF64, nil
	case 0x7b:
		return CoreValTypeV128, nil
	case 0x70:
		return CoreValTypeFuncRef, nil
	case 0x6f:
		return CoreValTypeExternRef, nil
	default:
		return 0, fmt.Errorf("unknown core value type: 0x%02x", b)
	}
}

// coreValTypeFromParam converts a CoreValParam to a CoreValType, validating ref types
// against the component state's core type index space.
func coreValTypeFromParam(p CoreValParam, cs *ComponentState) (CoreValType, error) {
	if p.Byte == 0x64 {
		// ref type - validate the type index
		ct, err := cs.getCoreType(p.RefIndex)
		if err != nil {
			return 0, fmt.Errorf("type index out of bounds")
		}
		if ct.Kind == CoreAnyTypeModule {
			return 0, fmt.Errorf("type index %d is a module type", p.RefIndex)
		}
		return CoreValTypeFuncRef, nil // treat as funcref for now
	}
	return coreValTypeFromByte(p.Byte)
}

// coreValTypeFromParamInModuleType converts a CoreValParam to a CoreValType,
// validating ref types against the module type state.
func coreValTypeFromParamInModuleType(p CoreValParam, ms *moduleTypeState) (CoreValType, error) {
	if p.Byte == 0x64 {
		// ref type - validate the type index against the module type's types
		if p.RefIndex >= uint32(len(ms.types)) {
			return 0, fmt.Errorf("type index out of bounds")
		}
		return CoreValTypeFuncRef, nil // treat as funcref for now
	}
	return coreValTypeFromByte(p.Byte)
}

func (v *Validator) validateCoreTypeSection(p *CoreTypeSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("core type section outside component")
	}
	for ct, err := range p.Items() {
		if err != nil {
			return err
		}
		if err := v.addCoreType(cs, ct); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) addCoreType(cs *ComponentState, ct CoreType) error {
	switch t := ct.(type) {
	case *CoreModuleType:
		return v.addCoreModuleType(cs, t)
	case *CoreFuncType:
		return v.addCoreFuncType(cs, t)
	default:
		return fmt.Errorf("unknown core type %T", ct)
	}
}

func (v *Validator) addCoreFuncType(cs *ComponentState, ft *CoreFuncType) error {
	// Convert to internal form
	params := make([]CoreValType, len(ft.Params))
	for i, p := range ft.Params {
		cvt, err := coreValTypeFromParam(p, cs)
		if err != nil {
			return err
		}
		params[i] = cvt
	}
	results := make([]CoreValType, len(ft.Results))
	for i, r := range ft.Results {
		cvt, err := coreValTypeFromParam(r, cs)
		if err != nil {
			return err
		}
		results[i] = cvt
	}
	id := v.arena.pushCoreFuncType(CoreFuncTypeDesc{Params: params, Results: results})
	cs.coreTypes = append(cs.coreTypes, CoreAnyTypeID{Kind: CoreAnyTypeFunc, Index: uint32(id)})
	return nil
}

func (v *Validator) addCoreModuleType(cs *ComponentState, mt *CoreModuleType) error {
	// Create internal module type from declarations
	modType := CoreModuleTypeDesc{
		Imports: make(map[importKey]CoreEntityType),
		Exports: make(map[string]CoreEntityType),
	}

	// We need a temporary state to track types within the module type
	tempState := &moduleTypeState{
		arena:      v.arena,
		types:      nil,
		imports:    make(map[importKey]CoreEntityType),
		exports:    make(map[string]CoreEntityType),
		parent:     cs,
		components: v.components,
	}

	for _, decl := range mt.Declarations {
		if err := tempState.addDeclaration(decl); err != nil {
			return err
		}
	}

	modType.Imports = tempState.imports
	modType.ImportOrder = tempState.importOrder
	modType.Exports = tempState.exports

	id := v.arena.pushCoreModuleType(modType)
	cs.coreTypes = append(cs.coreTypes, CoreAnyTypeID{Kind: CoreAnyTypeModule, Index: uint32(id)})
	return nil
}

// moduleTypeState tracks state while processing module type declarations.
type moduleTypeState struct {
	arena       *TypeArena
	types       []CoreFuncTypeID // sub-types within the module type
	imports     map[importKey]CoreEntityType
	importOrder []importKey // preserves insertion order
	exports     map[string]CoreEntityType
	parent      *ComponentState
	components  []*ComponentState // reference to the validator's component stack
}

func (ms *moduleTypeState) addDeclaration(decl ModuleTypeDeclaration) error {
	switch d := decl.(type) {
	case *ModuleDeclType:
		return ms.addType(d)
	case *ModuleDeclImport:
		return ms.addImport(d)
	case *ModuleDeclExport:
		return ms.addExport(d)
	case *ModuleDeclOuterAlias:
		return ms.addOuterAlias(d)
	default:
		return fmt.Errorf("unknown module type declaration %T", decl)
	}
}

func (ms *moduleTypeState) addType(d *ModuleDeclType) error {
	params := make([]CoreValType, len(d.Params))
	for i, p := range d.Params {
		cvt, err := coreValTypeFromParamInModuleType(p, ms)
		if err != nil {
			return err
		}
		params[i] = cvt
	}
	results := make([]CoreValType, len(d.Results))
	for i, r := range d.Results {
		cvt, err := coreValTypeFromParamInModuleType(r, ms)
		if err != nil {
			return err
		}
		results[i] = cvt
	}
	id := ms.arena.pushCoreFuncType(CoreFuncTypeDesc{Params: params, Results: results})
	ms.types = append(ms.types, id)
	return nil
}

func (ms *moduleTypeState) addImport(d *ModuleDeclImport) error {
	et, err := ms.resolveEntityType(d.EntityType)
	if err != nil {
		return err
	}
	key := importKey{Module: d.Module, Name: d.Name}
	if _, exists := ms.imports[key]; exists {
		return fmt.Errorf("duplicate import name `%s:%s`", d.Module, d.Name)
	}
	ms.imports[key] = et
	ms.importOrder = append(ms.importOrder, key)
	return nil
}

func (ms *moduleTypeState) addExport(d *ModuleDeclExport) error {
	et, err := ms.resolveEntityType(d.EntityType)
	if err != nil {
		return err
	}
	if _, exists := ms.exports[d.Name]; exists {
		return fmt.Errorf("export name `%s` already defined", d.Name)
	}
	ms.exports[d.Name] = et
	return nil
}

// resolveOuterCount resolves an outer alias count from within a module type.
// count=1 means the immediate parent component, count=2 means grandparent, etc.
func (ms *moduleTypeState) resolveOuterCount(count uint32) *ComponentState {
	stackLen := len(ms.components)
	// count=1 -> components[stackLen-1] (parent/current component)
	// count=2 -> components[stackLen-2] (grandparent)
	target := stackLen - int(count)
	if target < 0 {
		return nil
	}
	return ms.components[target]
}

func (ms *moduleTypeState) addOuterAlias(d *ModuleDeclOuterAlias) error {
	// Outer alias in module type: alias a core type from an enclosing scope
	// count=1 means the immediate parent component, count=2 means grandparent, etc.
	if d.Kind == OuterAliasKindCoreType {
		if d.Count == 0 {
			return fmt.Errorf("invalid outer alias count of 0")
		}
		// count=1 -> parent component (index 0 from end of component stack)
		// count=2 -> grandparent (index 1 from end), etc.
		target := ms.resolveOuterCount(d.Count)
		if target == nil {
			return fmt.Errorf("invalid outer alias count of %d", d.Count)
		}
		ct, err := target.getCoreType(d.Index)
		if err != nil {
			return err
		}
		if ct.Kind != CoreAnyTypeFunc {
			return fmt.Errorf("aliased core type is not a function type")
		}
		ms.types = append(ms.types, CoreFuncTypeID(ct.Index))
		return nil
	}
	return fmt.Errorf("unsupported outer alias kind in module type")
}

func (ms *moduleTypeState) resolveEntityType(et CoreEntityTypeRef) (CoreEntityType, error) {
	switch et.Kind {
	case CoreEntityFunc:
		if et.TypeIndex >= uint32(len(ms.types)) {
			return CoreEntityType{}, fmt.Errorf("type index out of bounds")
		}
		return CoreEntityType{Kind: CoreEntityFunc, Func: ms.types[et.TypeIndex]}, nil
	case CoreEntityTable:
		return CoreEntityType{Kind: CoreEntityTable, Table: et.Table}, nil
	case CoreEntityMemory:
		return CoreEntityType{Kind: CoreEntityMemory, Memory: et.Memory}, nil
	case CoreEntityGlobal:
		return CoreEntityType{Kind: CoreEntityGlobal, Global: et.Global}, nil
	default:
		return CoreEntityType{}, fmt.Errorf("unknown core entity kind")
	}
}

// ---------- Component Type Section ----------
