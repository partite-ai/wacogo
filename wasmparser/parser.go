package wasmparser

import (
	"bytes"
	"encoding/binary"
	"io"
	"iter"
)

// Encoding identifies whether a binary is a core module or a component.
type Encoding uint8

const (
	EncodingModule    Encoding = 0
	EncodingComponent Encoding = 1
)

// Range represents a byte range in the original binary stream.
type Range struct {
	Start uint64
	End   uint64
}

// Payload is the interface implemented by all parser output types.
type Payload interface {
	payload() // private marker
}

// ---- Payload types ----

// VersionPayload is emitted after reading the 8-byte header.
type VersionPayload struct {
	Encoding Encoding
	Num      uint32
	Range    Range
}

func (*VersionPayload) payload() {}

// EndPayload is emitted when the parser reaches the end of its data.
type EndPayload struct {
	Range Range
}

func (*EndPayload) payload() {}

// CustomSectionPayload is emitted for custom sections.
type CustomSectionPayload struct {
	Name  string
	Data  []byte
	Range Range
}

func (*CustomSectionPayload) payload() {}

// ModuleSectionPayload is emitted for nested core module sections (component section id 1).
type ModuleSectionPayload struct {
	Parser *Parser
	Data   []byte // complete core module binary (header + all sections)
	Range  Range
}

func (*ModuleSectionPayload) payload() {}

// ---- Core module section payloads ----

// ModuleTypeSectionPayload is emitted for core module type sections (id 1).
type ModuleTypeSectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleTypeSectionPayload) payload() {}

// Items returns an iterator over the function types in this section.
func (p *ModuleTypeSectionPayload) Items() iter.Seq2[*CoreFuncType, error] {
	return sectionItems(p.reader, readModuleCoreFuncType)
}

// ModuleImportSectionPayload is emitted for core module import sections (id 2).
type ModuleImportSectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleImportSectionPayload) payload() {}

// Items returns an iterator over the imports in this section.
func (p *ModuleImportSectionPayload) Items() iter.Seq2[ModuleImport, error] {
	return sectionItems(p.reader, readModuleImport)
}

// ModuleFunctionSectionPayload is emitted for core module function sections (id 3).
type ModuleFunctionSectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleFunctionSectionPayload) payload() {}

// Items returns an iterator over the type indices in this section.
func (p *ModuleFunctionSectionPayload) Items() iter.Seq2[uint32, error] {
	return sectionItems(p.reader, readModuleFuncTypeIndex)
}

// ModuleTableSectionPayload is emitted for core module table sections (id 4).
type ModuleTableSectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleTableSectionPayload) payload() {}

// Items returns an iterator over the table types in this section.
func (p *ModuleTableSectionPayload) Items() iter.Seq2[CoreTableType, error] {
	return sectionItems(p.reader, readModuleTableItem)
}

// ModuleMemorySectionPayload is emitted for core module memory sections (id 5).
type ModuleMemorySectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleMemorySectionPayload) payload() {}

// Items returns an iterator over the memory types in this section.
func (p *ModuleMemorySectionPayload) Items() iter.Seq2[CoreMemoryType, error] {
	return sectionItems(p.reader, readModuleMemoryItem)
}

// ModuleGlobalSectionPayload is emitted for core module global sections (id 6).
// Contains the raw section bytes (globals contain init exprs with complex encoding).
type ModuleGlobalSectionPayload struct {
	Data  []byte
	Range Range
}

func (*ModuleGlobalSectionPayload) payload() {}

// ModuleExportSectionPayload is emitted for core module export sections (id 7).
type ModuleExportSectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleExportSectionPayload) payload() {}

// Items returns an iterator over the exports in this section.
func (p *ModuleExportSectionPayload) Items() iter.Seq2[ModuleExport, error] {
	return sectionItems(p.reader, readModuleExport)
}

// ModuleStartSectionPayload is emitted for core module start sections (id 8).
type ModuleStartSectionPayload struct {
	FuncIndex uint32
	Range     Range
}

func (*ModuleStartSectionPayload) payload() {}

// ModuleElementSectionPayload is emitted for core module element sections (id 9).
// Contains the raw section bytes (element segments have complex encoding).
type ModuleElementSectionPayload struct {
	Data  []byte
	Range Range
}

func (*ModuleElementSectionPayload) payload() {}

// ModuleCodeSectionPayload is emitted for core module code sections (id 10).
// Contains the raw section bytes (function bodies).
type ModuleCodeSectionPayload struct {
	Data  []byte
	Range Range
}

func (*ModuleCodeSectionPayload) payload() {}

// ModuleDataSectionPayload is emitted for core module data sections (id 11).
// Contains the raw section bytes (data segments have complex encoding).
type ModuleDataSectionPayload struct {
	Data  []byte
	Range Range
}

func (*ModuleDataSectionPayload) payload() {}

// ModuleDataCountSectionPayload is emitted for core module data count sections (id 12).
type ModuleDataCountSectionPayload struct {
	Count uint32
	Range Range
}

func (*ModuleDataCountSectionPayload) payload() {}

// ModuleTagSectionPayload is emitted for core module tag sections (id 13).
type ModuleTagSectionPayload struct {
	reader *BinaryReader
	Range  Range
}

func (*ModuleTagSectionPayload) payload() {}

// ComponentSectionPayload is emitted for nested component sections (component section id 4).
type ComponentSectionPayload struct {
	Parser           *Parser
	ValidatingParser *ValidatingParser // non-nil when produced by a ValidatingParser
	Range            Range
}

func (*ComponentSectionPayload) payload() {}

// ComponentTypeSectionPayload is emitted for component type sections (id 7).
type ComponentTypeSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*ComponentTypeSectionPayload) payload() {}

// Items returns an iterator over the types in this section.
func (p *ComponentTypeSectionPayload) Items() iter.Seq2[ComponentTypeDef, error] {
	return sectionItems(p.reader, readComponentTypeDef)
}

// ComponentImportSectionPayload is emitted for component import sections (id 10).
type ComponentImportSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*ComponentImportSectionPayload) payload() {}

// Items returns an iterator over the imports in this section.
func (p *ComponentImportSectionPayload) Items() iter.Seq2[*ComponentImport, error] {
	return sectionItems(p.reader, unmarshalNew[*ComponentImport, ComponentImport])
}

// ComponentExportSectionPayload is emitted for component export sections (id 11).
type ComponentExportSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*ComponentExportSectionPayload) payload() {}

// Items returns an iterator over the exports in this section.
func (p *ComponentExportSectionPayload) Items() iter.Seq2[*ComponentExport, error] {
	return sectionItems(p.reader, unmarshalNew[*ComponentExport, ComponentExport])
}

// ComponentInstanceSectionPayload is emitted for component instance sections (id 5).
type ComponentInstanceSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*ComponentInstanceSectionPayload) payload() {}

// Items returns an iterator over the instances in this section.
func (p *ComponentInstanceSectionPayload) Items() iter.Seq2[ComponentInstance, error] {
	return sectionItems(p.reader, readComponentInstance)
}

// ComponentAliasSectionPayload is emitted for component alias sections (id 6).
type ComponentAliasSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*ComponentAliasSectionPayload) payload() {}

// Items returns an iterator over the aliases in this section.
func (p *ComponentAliasSectionPayload) Items() iter.Seq2[ComponentAlias, error] {
	return sectionItems(p.reader, readComponentAlias)
}

// ComponentCanonicalSectionPayload is emitted for canonical function sections (id 8).
type ComponentCanonicalSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*ComponentCanonicalSectionPayload) payload() {}

// Items returns an iterator over the canonical functions in this section.
func (p *ComponentCanonicalSectionPayload) Items() iter.Seq2[CanonicalFunction, error] {
	return sectionItems(p.reader, readCanonicalFunction)
}

// ComponentStartSectionPayload is emitted for start function sections (id 9).
type ComponentStartSectionPayload struct {
	Start ComponentStartFunction
	Range Range
}

func (*ComponentStartSectionPayload) payload() {}

// CoreTypeSectionPayload is emitted for core type sections (id 3).
type CoreTypeSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*CoreTypeSectionPayload) payload() {}

// Items returns an iterator over the core types in this section.
func (p *CoreTypeSectionPayload) Items() iter.Seq2[CoreType, error] {
	return sectionItems(p.reader, readCoreType)
}

// CoreInstanceSectionPayload is emitted for core instance sections (id 2).
type CoreInstanceSectionPayload struct {
	reader     *BinaryReader
	data       []byte
	dataOffset uint64
	Range      Range
}

func (*CoreInstanceSectionPayload) payload() {}

// Items returns an iterator over the core instances in this section.
func (p *CoreInstanceSectionPayload) Items() iter.Seq2[Instance, error] {
	return sectionItems(p.reader, readInstance)
}

// resettablePayload is implemented by section payloads whose BinaryReader
// may be consumed by validation and needs to be rewound before the caller
// iterates Items().
type resettablePayload interface {
	resetReader()
}

func (p *ComponentTypeSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *ComponentImportSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *ComponentExportSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *ComponentInstanceSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *ComponentAliasSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *ComponentCanonicalSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *CoreTypeSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}
func (p *CoreInstanceSectionPayload) resetReader() {
	p.reader = newBinaryReaderAt(bytes.NewReader(p.data), p.dataOffset)
}

// ---- Helper for section items ----

// sectionItems reads a u32 count from r, then yields that many items using readFn.
func sectionItems[T any](r *BinaryReader, readFn func(*BinaryReader) (T, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		count, err := r.ReadU32()
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		for range count {
			item, err := readFn(r)
			if !yield(item, err) {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

// ---- Parser state ----

type parserState uint8

const (
	stateHeader parserState = iota
	stateSectionStart
	stateEnd
)

// wasmMagic is the 4-byte magic number at the start of all WebAssembly binaries.
var wasmMagic = [4]byte{0x00, 0x61, 0x73, 0x6D}

// Component section IDs.
const (
	sectionCustom       = 0
	sectionCoreModule   = 1
	sectionCoreInstance = 2
	sectionCoreType     = 3
	sectionComponent    = 4
	sectionInstance     = 5
	sectionAlias        = 6
	sectionType         = 7
	sectionCanonical    = 8
	sectionStart        = 9
	sectionImport       = 10
	sectionExport       = 11
)

// Parser reads a WebAssembly binary and yields Payload values one at a time.
type Parser struct {
	reader          *BinaryReader
	state           parserState
	encoding        Encoding
	lastSectionRank int // canonical-order rank of the last non-custom core module section seen; 0 = none
}

// NewParser creates a new Parser that reads incrementally from r.
func NewParser(r io.Reader) *Parser {
	return &Parser{
		reader: NewBinaryReader(r),
		state:  stateHeader,
	}
}

// newParserFromBytes creates a parser from a byte slice at a given offset.
func newParserFromBytes(data []byte, offset uint64) *Parser {
	return &Parser{
		reader: newBinaryReaderAt(bytes.NewReader(data), offset),
		state:  stateHeader,
	}
}

// Next returns the next Payload from the binary. Returns io.EOF when done.
func (p *Parser) Next() (Payload, error) {
	switch p.state {
	case stateHeader:
		return p.readHeader()
	case stateSectionStart:
		return p.readSection()
	case stateEnd:
		return nil, io.EOF
	default:
		return nil, io.EOF
	}
}

func (p *Parser) readHeader() (Payload, error) {
	start := p.reader.Offset()

	// Read 8 bytes: 4 magic + 4 version/layer
	headerBytes, err := p.reader.ReadBytes(8)
	if err != nil {
		return nil, errfAt(start, "failed to read header: %v", err)
	}

	// Validate magic
	if headerBytes[0] != wasmMagic[0] || headerBytes[1] != wasmMagic[1] ||
		headerBytes[2] != wasmMagic[2] || headerBytes[3] != wasmMagic[3] {
		return nil, errAt(start, "invalid WebAssembly magic number")
	}

	// Parse version (LE u16) and layer (LE u16)
	version := binary.LittleEndian.Uint16(headerBytes[4:6])
	layer := binary.LittleEndian.Uint16(headerBytes[6:8])

	var enc Encoding
	switch {
	case version == 1 && layer == 0:
		enc = EncodingModule
	case version == 0x0d && layer == 1:
		enc = EncodingComponent
	default:
		return nil, errfAt(start+4, "unknown version %d layer %d", version, layer)
	}

	p.encoding = enc
	p.state = stateSectionStart

	return &VersionPayload{
		Encoding: enc,
		Num:      uint32(version),
		Range:    Range{Start: start, End: p.reader.Offset()},
	}, nil
}

func (p *Parser) readSection() (Payload, error) {
	if p.reader.IsEOF() {
		endOffset := p.reader.Offset()
		p.state = stateEnd
		return &EndPayload{
			Range: Range{Start: endOffset, End: endOffset},
		}, nil
	}

	start := p.reader.Offset()

	// Read section ID (1 byte, must not have high bit set)
	id, err := p.reader.ReadByte()
	if err != nil {
		return nil, errfAt(start, "failed to read section ID: %v", err)
	}
	if id&0x80 != 0 {
		return nil, errfAt(start, "section ID has high bit set: 0x%02x", id)
	}

	// Read section length (LEB128 u32)
	length, err := p.reader.ReadU32()
	if err != nil {
		return nil, errfAt(p.reader.Offset(), "failed to read section length: %v", err)
	}

	dataStart := p.reader.Offset()
	dataEnd := dataStart + uint64(length)
	rng := Range{Start: start, End: dataEnd}

	if p.encoding == EncodingComponent {
		return p.dispatchComponentSection(id, int64(length), dataStart, rng)
	}

	// Core module sections must appear in canonical order, with custom
	// sections allowed anywhere. Each non-custom section may appear at most
	// once, so a rank that is not strictly greater than the previous one is
	// an out-of-order error.
	if id != moduleSectionCustom {
		if rank := coreSectionRank(id); rank != 0 {
			if rank <= p.lastSectionRank {
				return nil, errAt(start, "section out of order")
			}
			p.lastSectionRank = rank
		}
	}

	// Core module sections
	sectionData, err := p.reader.ReadBytes(int(length))
	if err != nil {
		return nil, errfAt(dataStart, "section data truncated: %v", err)
	}

	return p.dispatchModuleSection(id, sectionData, dataStart, rng)
}

func (p *Parser) dispatchComponentSection(id byte, length int64, dataOffset uint64, rng Range) (Payload, error) {
	switch id {
	case sectionCoreModule, sectionComponent:
		// Nested module/component: read all section bytes, then create a sub-parser
		// from those bytes. The top-level read is still incremental (one section at
		// a time), but each nested section's content is fully read before parsing.
		sectionData, err := p.reader.ReadBytes(int(length))
		if err != nil {
			return nil, errfAt(dataOffset, "section data truncated: %v", err)
		}
		sub := newParserFromBytes(sectionData, dataOffset)

		if id == sectionCoreModule {
			return &ModuleSectionPayload{
				Parser: sub,
				Data:   sectionData,
				Range:  rng,
			}, nil
		}
		return &ComponentSectionPayload{
			Parser: sub,
			Range:  rng,
		}, nil

	default:
		// For all other sections, read the section data fully into a byte slice,
		// then create a byte-slice BinaryReader for Items() iteration.
		sectionData, err := p.reader.ReadBytes(int(length))
		if err != nil {
			return nil, errfAt(dataOffset, "section data truncated: %v", err)
		}

		return p.dispatchComponentSectionFromData(id, sectionData, dataOffset, rng)
	}
}

func (p *Parser) dispatchComponentSectionFromData(id byte, data []byte, dataOffset uint64, rng Range) (Payload, error) {
	switch id {
	case sectionCustom:
		// Custom section: read name then remaining is data
		sr := newBinaryReaderAt(bytes.NewReader(data), dataOffset)
		name, err := sr.ReadString()
		if err != nil {
			return nil, err
		}
		// The name was length-prefixed within data; the rest is custom section payload.
		nameConsumed := sr.Offset() - dataOffset
		remaining := data[nameConsumed:]
		return &CustomSectionPayload{
			Name:  name,
			Data:  remaining,
			Range: rng,
		}, nil

	case sectionCoreInstance:
		return &CoreInstanceSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionCoreType:
		return &CoreTypeSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionInstance:
		return &ComponentInstanceSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionAlias:
		return &ComponentAliasSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionType:
		return &ComponentTypeSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionCanonical:
		return &ComponentCanonicalSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionStart:
		sr := newBinaryReaderAt(bytes.NewReader(data), dataOffset)
		var start ComponentStartFunction
		if err := start.unmarshalBinary(sr); err != nil {
			return nil, err
		}
		return &ComponentStartSectionPayload{
			Start: start,
			Range: rng,
		}, nil

	case sectionImport:
		return &ComponentImportSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	case sectionExport:
		return &ComponentExportSectionPayload{
			reader:     newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			data:       data,
			dataOffset: dataOffset,
			Range:      rng,
		}, nil

	default:
		return nil, errfAt(rng.Start, "unknown component section ID: %d", id)
	}
}

// Core module section IDs.
const (
	moduleSectionCustom    byte = 0
	moduleSectionType      byte = 1
	moduleSectionImport    byte = 2
	moduleSectionFunction  byte = 3
	moduleSectionTable     byte = 4
	moduleSectionMemory    byte = 5
	moduleSectionGlobal    byte = 6
	moduleSectionExport    byte = 7
	moduleSectionStart     byte = 8
	moduleSectionElement   byte = 9
	moduleSectionCode      byte = 10
	moduleSectionData      byte = 11
	moduleSectionDataCount byte = 12
	moduleSectionTag       byte = 13
)

// coreSectionRank returns the canonical-order position of a core module
// section id. The data count section is placed before code per bulk-memory,
// and the tag section between memory and global per exception-handling.
// Custom (0) and unrecognized ids return 0 and are not order-checked.
func coreSectionRank(id byte) int {
	switch id {
	case moduleSectionType:
		return 1
	case moduleSectionImport:
		return 2
	case moduleSectionFunction:
		return 3
	case moduleSectionTable:
		return 4
	case moduleSectionMemory:
		return 5
	case moduleSectionTag:
		return 6
	case moduleSectionGlobal:
		return 7
	case moduleSectionExport:
		return 8
	case moduleSectionStart:
		return 9
	case moduleSectionElement:
		return 10
	case moduleSectionDataCount:
		return 11
	case moduleSectionCode:
		return 12
	case moduleSectionData:
		return 13
	}
	return 0
}

func (p *Parser) dispatchModuleSection(id byte, data []byte, dataOffset uint64, rng Range) (Payload, error) {
	switch id {
	case moduleSectionCustom:
		sr := newBinaryReaderAt(bytes.NewReader(data), dataOffset)
		name, err := sr.ReadString()
		if err != nil {
			return nil, err
		}
		nameConsumed := sr.Offset() - dataOffset
		remaining := data[nameConsumed:]
		return &CustomSectionPayload{
			Name:  name,
			Data:  remaining,
			Range: rng,
		}, nil

	case moduleSectionType:
		return &ModuleTypeSectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	case moduleSectionImport:
		return &ModuleImportSectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	case moduleSectionFunction:
		return &ModuleFunctionSectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	case moduleSectionTable:
		return &ModuleTableSectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	case moduleSectionMemory:
		return &ModuleMemorySectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	case moduleSectionGlobal:
		return &ModuleGlobalSectionPayload{
			Data:  data,
			Range: rng,
		}, nil

	case moduleSectionExport:
		return &ModuleExportSectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	case moduleSectionStart:
		sr := newBinaryReaderAt(bytes.NewReader(data), dataOffset)
		funcIdx, err := sr.ReadU32()
		if err != nil {
			return nil, err
		}
		return &ModuleStartSectionPayload{
			FuncIndex: funcIdx,
			Range:     rng,
		}, nil

	case moduleSectionElement:
		return &ModuleElementSectionPayload{
			Data:  data,
			Range: rng,
		}, nil

	case moduleSectionCode:
		return &ModuleCodeSectionPayload{
			Data:  data,
			Range: rng,
		}, nil

	case moduleSectionData:
		return &ModuleDataSectionPayload{
			Data:  data,
			Range: rng,
		}, nil

	case moduleSectionDataCount:
		sr := newBinaryReaderAt(bytes.NewReader(data), dataOffset)
		count, err := sr.ReadU32()
		if err != nil {
			return nil, err
		}
		return &ModuleDataCountSectionPayload{
			Count: count,
			Range: rng,
		}, nil

	case moduleSectionTag:
		return &ModuleTagSectionPayload{
			reader: newBinaryReaderAt(bytes.NewReader(data), dataOffset),
			Range:  rng,
		}, nil

	default:
		// Unknown section IDs in modules: return as custom section with empty name
		return &CustomSectionPayload{
			Name:  "",
			Data:  data,
			Range: rng,
		}, nil
	}
}
