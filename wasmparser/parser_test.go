package wasmparser

import (
	"bytes"
	"io"
	"testing"
)

var emptyComponent = []byte{
	0x00, 0x61, 0x73, 0x6D, // magic
	0x0d, 0x00, 0x01, 0x00, // version 13, layer 1
}

var emptyModule = []byte{
	0x00, 0x61, 0x73, 0x6D, // magic
	0x01, 0x00, 0x00, 0x00, // version 1, layer 0
}

func TestParserEmptyComponent(t *testing.T) {
	p := NewParser(bytes.NewReader(emptyComponent))

	// First payload: VersionPayload
	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vp, ok := payload.(*VersionPayload)
	if !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}
	if vp.Encoding != EncodingComponent {
		t.Errorf("expected EncodingComponent, got %v", vp.Encoding)
	}
	if vp.Num != 0x0d {
		t.Errorf("expected version 13, got %d", vp.Num)
	}

	// Second payload: EndPayload
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = payload.(*EndPayload)
	if !ok {
		t.Fatalf("expected *EndPayload, got %T", payload)
	}

	// Third call: EOF
	_, err = p.Next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestParserEmptyModule(t *testing.T) {
	p := NewParser(bytes.NewReader(emptyModule))

	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vp, ok := payload.(*VersionPayload)
	if !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}
	if vp.Encoding != EncodingModule {
		t.Errorf("expected EncodingModule, got %v", vp.Encoding)
	}
	if vp.Num != 1 {
		t.Errorf("expected version 1, got %d", vp.Num)
	}

	// End
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = payload.(*EndPayload)
	if !ok {
		t.Fatalf("expected *EndPayload, got %T", payload)
	}
}

func TestParserInvalidMagic(t *testing.T) {
	bad := []byte{0x00, 0x61, 0x73, 0x00, 0x01, 0x00, 0x00, 0x00}
	p := NewParser(bytes.NewReader(bad))
	_, err := p.Next()
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
}

func TestParserComponentWithTypeSection(t *testing.T) {
	// Component header + type section (id=7) with 0 types
	data := make([]byte, 0, 20)
	data = append(data, emptyComponent...)
	data = append(data, 0x07) // section id: type
	data = append(data, 0x01) // section length: 1 byte
	data = append(data, 0x00) // count: 0 types

	p := NewParser(bytes.NewReader(data))

	// Version
	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := payload.(*VersionPayload)
	if !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// Type section
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ts, ok := payload.(*ComponentTypeSectionPayload)
	if !ok {
		t.Fatalf("expected *ComponentTypeSectionPayload, got %T", payload)
	}

	// Iterate items - should be 0
	count := 0
	for _, err := range ts.Items() {
		if err != nil {
			t.Fatalf("unexpected error iterating: %v", err)
		}
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 items, got %d", count)
	}

	// End
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok = payload.(*EndPayload)
	if !ok {
		t.Fatalf("expected *EndPayload, got %T", payload)
	}
}

func TestParserComponentWithImportSection(t *testing.T) {
	// Component header + import section (id=10) with 0 imports
	data := make([]byte, 0, 20)
	data = append(data, emptyComponent...)
	data = append(data, 0x0a) // section id: import
	data = append(data, 0x01) // section length: 1 byte
	data = append(data, 0x00) // count: 0

	p := NewParser(bytes.NewReader(data))

	// Version
	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// Import section
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	is, ok := payload.(*ComponentImportSectionPayload)
	if !ok {
		t.Fatalf("expected *ComponentImportSectionPayload, got %T", payload)
	}

	count := 0
	for _, err := range is.Items() {
		if err != nil {
			t.Fatalf("unexpected error iterating: %v", err)
		}
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 items, got %d", count)
	}
}

func TestParserNestedComponent(t *testing.T) {
	// Outer component containing an inner empty component
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer header
		0x04,                                             // section id: component
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // inner header
	}

	p := NewParser(bytes.NewReader(data))

	// Outer version
	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// ComponentSectionPayload with sub-parser
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cs, ok := payload.(*ComponentSectionPayload)
	if !ok {
		t.Fatalf("expected *ComponentSectionPayload, got %T", payload)
	}

	// Use the sub-parser to read inner component
	innerPayload, err := cs.Parser.Next()
	if err != nil {
		t.Fatalf("unexpected error from inner parser: %v", err)
	}
	vp, ok := innerPayload.(*VersionPayload)
	if !ok {
		t.Fatalf("expected inner *VersionPayload, got %T", innerPayload)
	}
	if vp.Encoding != EncodingComponent {
		t.Errorf("expected inner EncodingComponent, got %v", vp.Encoding)
	}

	// Inner end
	innerPayload, err = cs.Parser.Next()
	if err != nil {
		t.Fatalf("unexpected error from inner parser: %v", err)
	}
	if _, ok := innerPayload.(*EndPayload); !ok {
		t.Fatalf("expected inner *EndPayload, got %T", innerPayload)
	}

	// Outer end
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*EndPayload); !ok {
		t.Fatalf("expected *EndPayload, got %T", payload)
	}
}

func TestParserNestedModule(t *testing.T) {
	// Outer component containing an inner empty module
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer header
		0x01,                                             // section id: core module
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00, // inner module header
	}

	p := NewParser(bytes.NewReader(data))

	// Outer version
	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// ModuleSectionPayload with sub-parser
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ms, ok := payload.(*ModuleSectionPayload)
	if !ok {
		t.Fatalf("expected *ModuleSectionPayload, got %T", payload)
	}

	// Inner version
	innerPayload, err := ms.Parser.Next()
	if err != nil {
		t.Fatalf("unexpected error from inner parser: %v", err)
	}
	vp, ok := innerPayload.(*VersionPayload)
	if !ok {
		t.Fatalf("expected inner *VersionPayload, got %T", innerPayload)
	}
	if vp.Encoding != EncodingModule {
		t.Errorf("expected inner EncodingModule, got %v", vp.Encoding)
	}

	// Inner end
	innerPayload, err = ms.Parser.Next()
	if err != nil {
		t.Fatalf("unexpected error from inner parser: %v", err)
	}
	if _, ok := innerPayload.(*EndPayload); !ok {
		t.Fatalf("expected inner *EndPayload, got %T", innerPayload)
	}

	// Outer end
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*EndPayload); !ok {
		t.Fatalf("expected *EndPayload, got %T", payload)
	}
}

func TestParserCustomSection(t *testing.T) {
	// Component header + custom section (id=0) with name "test" and some data
	name := "test"
	sectionData := []byte{0xCA, 0xFE}
	// custom section content: name length (LEB128) + name + data
	content := []byte{byte(len(name))}
	content = append(content, []byte(name)...)
	content = append(content, sectionData...)

	data := make([]byte, 0, 30)
	data = append(data, emptyComponent...)
	data = append(data, 0x00)              // section id: custom
	data = append(data, byte(len(content))) // section length
	data = append(data, content...)

	p := NewParser(bytes.NewReader(data))

	// Version
	payload, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// Custom section
	payload, err = p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cs, ok := payload.(*CustomSectionPayload)
	if !ok {
		t.Fatalf("expected *CustomSectionPayload, got %T", payload)
	}
	if cs.Name != "test" {
		t.Errorf("expected name 'test', got %q", cs.Name)
	}
	if !bytes.Equal(cs.Data, sectionData) {
		t.Errorf("expected data %v, got %v", sectionData, cs.Data)
	}
}

func TestParserSectionHighBit(t *testing.T) {
	// Section ID with high bit set should be an error
	data := make([]byte, 0, 20)
	data = append(data, emptyComponent...)
	data = append(data, 0x80) // section id with high bit set
	data = append(data, 0x01) // section length: 1
	data = append(data, 0x00) // dummy data

	p := NewParser(bytes.NewReader(data))

	// Version
	_, err := p.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should error on high bit section ID
	_, err = p.Next()
	if err == nil {
		t.Fatal("expected error for section ID with high bit set")
	}
}
