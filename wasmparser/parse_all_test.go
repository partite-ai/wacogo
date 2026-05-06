package wasmparser

import (
	"bytes"
	"io"
	"testing"
)

func TestParseAll(t *testing.T) {
	var payloads []Payload
	for p, err := range ParseAll(bytes.NewReader(emptyComponent)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		payloads = append(payloads, p)
	}
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if _, ok := payloads[0].(*VersionPayload); !ok {
		t.Errorf("expected *VersionPayload at 0, got %T", payloads[0])
	}
	if _, ok := payloads[1].(*EndPayload); !ok {
		t.Errorf("expected *EndPayload at 1, got %T", payloads[1])
	}
}

func TestParseAllNestedComponent(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer header
		0x04,                                             // section id: component
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // inner header
	}

	var payloads []Payload
	for p, err := range ParseAll(bytes.NewReader(data)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		payloads = append(payloads, p)
	}

	// Expected: outer version, component section, inner version, inner end, outer end
	// Wait - the ComponentSectionPayload is yielded, then its sub-parser is pushed.
	// The sub-parser yields: inner version, inner end.
	// Then the sub-parser returns EOF, gets popped.
	// The outer parser continues: outer end.
	// So: outer version, component section payload, inner version, inner end, outer end = 5 payloads.
	if len(payloads) != 5 {
		t.Fatalf("expected 5 payloads, got %d", len(payloads))
	}

	// Outer version
	vp0, ok := payloads[0].(*VersionPayload)
	if !ok {
		t.Fatalf("expected *VersionPayload at 0, got %T", payloads[0])
	}
	if vp0.Encoding != EncodingComponent {
		t.Errorf("expected outer EncodingComponent")
	}

	// Component section
	if _, ok := payloads[1].(*ComponentSectionPayload); !ok {
		t.Fatalf("expected *ComponentSectionPayload at 1, got %T", payloads[1])
	}

	// Inner version
	vp2, ok := payloads[2].(*VersionPayload)
	if !ok {
		t.Fatalf("expected *VersionPayload at 2, got %T", payloads[2])
	}
	if vp2.Encoding != EncodingComponent {
		t.Errorf("expected inner EncodingComponent")
	}

	// Inner end
	if _, ok := payloads[3].(*EndPayload); !ok {
		t.Fatalf("expected *EndPayload at 3, got %T", payloads[3])
	}

	// Outer end
	if _, ok := payloads[4].(*EndPayload); !ok {
		t.Fatalf("expected *EndPayload at 4, got %T", payloads[4])
	}
}

func TestParseAllNestedModule(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer component header
		0x01,                                             // section id: core module
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00, // inner module header
	}

	var payloads []Payload
	for p, err := range ParseAll(bytes.NewReader(data)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		payloads = append(payloads, p)
	}

	// outer version, module section, inner version, inner end, outer end = 5
	if len(payloads) != 5 {
		t.Fatalf("expected 5 payloads, got %d", len(payloads))
	}

	if _, ok := payloads[1].(*ModuleSectionPayload); !ok {
		t.Fatalf("expected *ModuleSectionPayload at 1, got %T", payloads[1])
	}

	vp, ok := payloads[2].(*VersionPayload)
	if !ok {
		t.Fatalf("expected *VersionPayload at 2, got %T", payloads[2])
	}
	if vp.Encoding != EncodingModule {
		t.Errorf("expected inner EncodingModule")
	}
}

func TestValidatingParserEmptyComponentIntegration(t *testing.T) {
	vp := NewValidatingParser(bytes.NewReader(emptyComponent), DefaultFeatures())
	count := 0
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 payloads, got %d", count)
	}
}

func TestValidatingParserNestedComponentIntegration(t *testing.T) {
	nested := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer
		0x04, 0x08, // section: component, len 8
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // inner
	}

	vp := NewValidatingParser(bytes.NewReader(nested), DefaultFeatures())
	count := 0
	for {
		p, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		if cs, ok := p.(*ComponentSectionPayload); ok {
			sub := cs.ValidatingParser
			for {
				_, err := sub.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				count++
			}
		}
	}
	// outer version + component section + inner version + inner end + outer end = 5
	if count != 5 {
		t.Fatalf("expected 5 payloads, got %d", count)
	}
}

func TestValidatingParserRejectsInvalidIntegration(t *testing.T) {
	// Component with an import section containing an import with an out-of-bounds type index
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // header
		0x0a,                                     // section: import (10)
		0x0a,                                     // section len: 10
		0x01,                                     // 1 import
		0x00, 0x03, 'f', 'o', 'o',               // plain name "foo"
		0x01,                                     // func type ref
		0xFF, 0xFF, 0x03,                         // type index 65535 (out of bounds)
	}

	var gotErr error
	vp := NewValidatingParser(bytes.NewReader(data), DefaultFeatures())
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Fatal("expected validation error for out-of-bounds type index")
	}
}

func TestParseAllEarlyBreak(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer
		0x04, 0x08,                                       // component section, len 8
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // inner
	}

	count := 0
	for _, err := range ParseAll(bytes.NewReader(data)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		if count == 2 {
			break // early exit after component section payload
		}
	}
	if count != 2 {
		t.Errorf("expected 2 iterations before break, got %d", count)
	}
}
