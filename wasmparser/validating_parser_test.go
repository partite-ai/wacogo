package wasmparser

import (
	"bytes"
	"io"
	"testing"
)

func TestValidatingParserEmptyComponent(t *testing.T) {
	vp := NewValidatingParser(bytes.NewReader(emptyComponent), DefaultFeatures())

	// First payload: VersionPayload
	payload, err := vp.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// Second payload: EndPayload
	payload, err = vp.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := payload.(*EndPayload); !ok {
		t.Fatalf("expected *EndPayload, got %T", payload)
	}

	// Third call: io.EOF
	_, err = vp.Next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Types() should succeed after EOF
	if _, err := vp.Types(); err != nil {
		t.Fatalf("Types() error: %v", err)
	}
}

func TestValidatingParserNestedComponent(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer header
		0x04,                                             // section id: component
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // inner header
	}

	outer := NewValidatingParser(bytes.NewReader(data), DefaultFeatures())

	// Outer VersionPayload
	payload, err := outer.Next()
	if err != nil {
		t.Fatalf("outer: unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("outer: expected *VersionPayload, got %T", payload)
	}

	// ComponentSectionPayload
	payload, err = outer.Next()
	if err != nil {
		t.Fatalf("outer: unexpected error: %v", err)
	}
	comp, ok := payload.(*ComponentSectionPayload)
	if !ok {
		t.Fatalf("outer: expected *ComponentSectionPayload, got %T", payload)
	}

	// Recurse into inner component via the automatically-populated ValidatingParser.
	inner := comp.ValidatingParser

	// Inner VersionPayload
	payload, err = inner.Next()
	if err != nil {
		t.Fatalf("inner: unexpected error: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("inner: expected *VersionPayload, got %T", payload)
	}

	// Inner EndPayload
	payload, err = inner.Next()
	if err != nil {
		t.Fatalf("inner: unexpected error: %v", err)
	}
	if _, ok := payload.(*EndPayload); !ok {
		t.Fatalf("inner: expected *EndPayload, got %T", payload)
	}

	// Inner EOF
	_, err = inner.Next()
	if err != io.EOF {
		t.Fatalf("inner: expected io.EOF, got %v", err)
	}

	// Outer EndPayload
	payload, err = outer.Next()
	if err != nil {
		t.Fatalf("outer: unexpected error: %v", err)
	}
	if _, ok := payload.(*EndPayload); !ok {
		t.Fatalf("outer: expected *EndPayload, got %T", payload)
	}

	// Outer EOF
	_, err = outer.Next()
	if err != io.EOF {
		t.Fatalf("outer: expected io.EOF, got %v", err)
	}

	// Types() should succeed
	if _, err := outer.Types(); err != nil {
		t.Fatalf("Types() error: %v", err)
	}
}

func TestValidatingParserRejectsInvalid(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // header
		0x0a,                                             // section: import (10)
		0x0a,                                             // section len: 10
		0x01,                                             // 1 import
		0x00, 0x03, 'f', 'o', 'o',                       // plain name "foo"
		0x01,                                             // func type ref
		0xFF, 0xFF, 0x03,                                 // type index 65535 (out of bounds)
	}

	vp := NewValidatingParser(bytes.NewReader(data), DefaultFeatures())

	// First payload: VersionPayload — should succeed
	payload, err := vp.Next()
	if err != nil {
		t.Fatalf("unexpected error on VersionPayload: %v", err)
	}
	if _, ok := payload.(*VersionPayload); !ok {
		t.Fatalf("expected *VersionPayload, got %T", payload)
	}

	// Second call should return a validation error for the out-of-bounds type index
	_, err = vp.Next()
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	if err == io.EOF {
		t.Fatal("expected a validation error, got io.EOF")
	}
}
