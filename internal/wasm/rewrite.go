package wasm

import (
	"bytes"
	"fmt"
)

// RewriteBlankImportNames rewrites any blank ("") import module names in a
// core wasm module binary with a synthetic name that doesn't collide with
// existing import module names.
//
// Returns:
//   - rewritten module bytes (or original if no blank names found)
//   - the synthetic name used (empty string if no rewriting needed)
//   - error if the binary is malformed
func RewriteBlankImportNames(wasm []byte) ([]byte, string, error) {
	const (
		wasmMagicSize   = 4
		wasmVersionSize = 4
		wasmHeaderSize  = wasmMagicSize + wasmVersionSize
	)

	if len(wasm) < wasmHeaderSize {
		return nil, "", fmt.Errorf("wasm binary too short: %d bytes", len(wasm))
	}

	// Verify magic and version
	if !bytes.Equal(wasm[:4], []byte{0x00, 0x61, 0x73, 0x6d}) {
		return nil, "", fmt.Errorf("invalid wasm magic")
	}
	if !bytes.Equal(wasm[4:8], []byte{0x01, 0x00, 0x00, 0x00}) {
		return nil, "", fmt.Errorf("invalid wasm version")
	}

	// Scan sections to find the import section (id=2)
	pos := wasmHeaderSize
	importSectionStart := -1
	importSectionDataStart := -1
	importSectionEnd := -1

	for pos < len(wasm) {
		sectionStart := pos
		if pos >= len(wasm) {
			break
		}
		sectionID := wasm[pos]
		pos++

		size, n, err := readU32LEB(wasm, pos)
		if err != nil {
			return nil, "", fmt.Errorf("reading section size at offset %d: %w", pos, err)
		}
		pos += n

		if int(size) > len(wasm)-pos {
			return nil, "", fmt.Errorf("section size %d exceeds remaining bytes at offset %d", size, pos)
		}

		if sectionID == 2 { // import section
			importSectionStart = sectionStart
			importSectionDataStart = pos
			importSectionEnd = pos + int(size)
			break
		}

		pos += int(size)
	}

	// No import section found
	if importSectionStart == -1 {
		return wasm, "", nil
	}

	// Parse the import section to collect module names and check for blanks
	importData := wasm[importSectionDataStart:importSectionEnd]
	existingNames, hasBlank, err := scanImportModuleNames(importData)
	if err != nil {
		return nil, "", fmt.Errorf("scanning import section: %w", err)
	}

	// No blank names found
	if !hasBlank {
		return wasm, "", nil
	}

	// Pick a collision-free synthetic name
	synthetic := ""
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("__wacogo_%d", i)
		if !existingNames[candidate] {
			synthetic = candidate
			break
		}
	}

	// Rebuild the import section with blank names replaced
	newImportData, err := rewriteImportSection(importData, synthetic)
	if err != nil {
		return nil, "", fmt.Errorf("rewriting import section: %w", err)
	}

	// Reassemble the module: header + sections before import + new import section + sections after
	var out bytes.Buffer
	out.Write(wasm[:importSectionStart]) // everything up to (but not including) the import section

	// Write new import section: id + size + data
	out.WriteByte(2)
	encodeU32(&out, uint32(len(newImportData)))
	out.Write(newImportData)

	out.Write(wasm[importSectionEnd:]) // everything after the import section

	return out.Bytes(), synthetic, nil
}

// scanImportModuleNames parses the import section data (without the section id/size header)
// and returns a set of all module names found and whether any are blank.
func scanImportModuleNames(data []byte) (map[string]bool, bool, error) {
	pos := 0
	count, n, err := readU32LEB(data, pos)
	if err != nil {
		return nil, false, fmt.Errorf("reading import count: %w", err)
	}
	pos += n

	names := make(map[string]bool)
	hasBlank := false

	for i := range count {
		// Read module name
		modName, nn, err := readName(data, pos)
		if err != nil {
			return nil, false, fmt.Errorf("reading module name for import %d: %w", i, err)
		}
		pos += nn

		if modName == "" {
			hasBlank = true
		} else {
			names[modName] = true
		}

		// Read field name (skip)
		_, nn, err = readName(data, pos)
		if err != nil {
			return nil, false, fmt.Errorf("reading field name for import %d: %w", i, err)
		}
		pos += nn

		// Read and skip import descriptor
		nn, err = skipImportDesc(data, pos)
		if err != nil {
			return nil, false, fmt.Errorf("skipping import descriptor for import %d: %w", i, err)
		}
		pos += nn
	}

	return names, hasBlank, nil
}

// rewriteImportSection returns a new import section data payload with blank module
// names replaced by synthetic.
func rewriteImportSection(data []byte, synthetic string) ([]byte, error) {
	pos := 0
	count, n, err := readU32LEB(data, pos)
	if err != nil {
		return nil, fmt.Errorf("reading import count: %w", err)
	}
	pos += n

	var out bytes.Buffer
	encodeU32(&out, count)

	for i := range count {
		// Read module name
		modName, nn, err := readName(data, pos)
		if err != nil {
			return nil, fmt.Errorf("reading module name for import %d: %w", i, err)
		}
		pos += nn

		// Write module name (replacing blank with synthetic)
		if modName == "" {
			encodeName(&out, synthetic)
		} else {
			encodeName(&out, modName)
		}

		// Read field name and write it verbatim
		_, nn, err = readName(data, pos)
		if err != nil {
			return nil, fmt.Errorf("reading field name for import %d: %w", i, err)
		}
		// Write the raw bytes of the name (length + bytes) verbatim
		out.Write(data[pos : pos+nn])
		pos += nn

		// Read and write the import descriptor verbatim
		nn, err = skipImportDesc(data, pos)
		if err != nil {
			return nil, fmt.Errorf("skipping import descriptor for import %d: %w", i, err)
		}
		out.Write(data[pos : pos+nn])
		pos += nn
	}

	return out.Bytes(), nil
}

// readU32LEB reads an unsigned LEB128 u32 from data starting at pos.
// Returns the value, number of bytes consumed, and any error.
func readU32LEB(data []byte, pos int) (uint32, int, error) {
	var result uint32
	var shift uint
	for i := range 5 {
		if pos+i >= len(data) {
			return 0, 0, fmt.Errorf("unexpected end of data reading LEB128 at offset %d", pos)
		}
		b := data[pos+i]
		result |= uint32(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			return result, i + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("LEB128 too long at offset %d", pos)
}

// readName reads a length-prefixed name from data at pos.
// Returns the name string, the total bytes consumed (length prefix + string bytes), and any error.
func readName(data []byte, pos int) (string, int, error) {
	length, n, err := readU32LEB(data, pos)
	if err != nil {
		return "", 0, fmt.Errorf("reading name length: %w", err)
	}
	end := pos + n + int(length)
	if end > len(data) {
		return "", 0, fmt.Errorf("name extends beyond data: need %d bytes, have %d", end, len(data))
	}
	return string(data[pos+n : end]), n + int(length), nil
}

// skipImportDesc skips past an import descriptor in data at pos.
// Returns number of bytes consumed and any error.
func skipImportDesc(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, fmt.Errorf("unexpected end of data reading import kind at offset %d", pos)
	}
	kind := data[pos]
	pos++
	consumed := 1

	switch kind {
	case 0x00: // func: typeidx (u32 LEB128)
		_, n, err := readU32LEB(data, pos)
		if err != nil {
			return 0, fmt.Errorf("reading func typeidx: %w", err)
		}
		consumed += n

	case 0x01: // table: reftype(1 byte) + limits
		if pos >= len(data) {
			return 0, fmt.Errorf("unexpected end reading table reftype")
		}
		consumed++ // reftype byte
		n, err := skipLimits(data, pos+1)
		if err != nil {
			return 0, fmt.Errorf("reading table limits: %w", err)
		}
		consumed += n

	case 0x02: // memory: limits
		n, err := skipLimits(data, pos)
		if err != nil {
			return 0, fmt.Errorf("reading memory limits: %w", err)
		}
		consumed += n

	case 0x03: // global: valtype(1 byte) + mut(1 byte)
		if pos+1 >= len(data) {
			return 0, fmt.Errorf("unexpected end reading global valtype/mut")
		}
		consumed += 2 // valtype + mut

	default:
		return 0, fmt.Errorf("unknown import kind 0x%02x at offset %d", kind, pos-1)
	}

	return consumed, nil
}

// skipLimits skips a limits structure in data at pos.
// Returns number of bytes consumed and any error.
func skipLimits(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, fmt.Errorf("unexpected end reading limits flag")
	}
	flag := data[pos]
	consumed := 1
	pos++

	// Read min
	_, n, err := readU32LEB(data, pos)
	if err != nil {
		return 0, fmt.Errorf("reading limits min: %w", err)
	}
	consumed += n
	pos += n

	if flag == 1 {
		// Read max
		_, n, err = readU32LEB(data, pos)
		if err != nil {
			return 0, fmt.Errorf("reading limits max: %w", err)
		}
		consumed += n
	}

	return consumed, nil
}
