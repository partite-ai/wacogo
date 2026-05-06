package wasmparser

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func wat2wasm(t *testing.T, wat string) []byte {
	t.Helper()
	cmd := exec.Command("wasm-tools", "parse", "-")
	cmd.Stdin = strings.NewReader(wat)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("wasm-tools parse: %s\n%s", err, ee.Stderr)
		}
		t.Fatalf("wasm-tools parse: %v", err)
	}
	return out
}

func TestValidatorEmptyComponent(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	for payload, err := range ParseAll(bytes.NewReader(emptyComponent)) {
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidatePayload(payload); err != nil {
			t.Fatal(err)
		}
	}
	_, err := v.Types()
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatorNestedComponent(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer header
		0x04,                                             // section id: component
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // inner header
	}

	v := NewValidator(DefaultFeatures())
	for payload, err := range ParseAll(bytes.NewReader(data)) {
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidatePayload(payload); err != nil {
			t.Fatal(err)
		}
	}
	_, err := v.Types()
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatorNestedModule(t *testing.T) {
	data := []byte{
		0x00, 0x61, 0x73, 0x6D, 0x0d, 0x00, 0x01, 0x00, // outer component header
		0x01,                                             // section id: core module
		0x08,                                             // section length: 8
		0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00, // inner module header
	}

	v := NewValidator(DefaultFeatures())
	for payload, err := range ParseAll(bytes.NewReader(data)) {
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidatePayload(payload); err != nil {
			t.Fatal(err)
		}
	}
	_, err := v.Types()
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatorTypesBeforeEnd(t *testing.T) {
	v := NewValidator(DefaultFeatures())
	// Don't process any payloads — Types() should fail.
	_, err := v.Types()
	if err == nil {
		t.Fatal("expected error calling Types() before end")
	}
}

func TestValidatorParseAndValidateEmpty(t *testing.T) {
	vp := NewValidatingParser(bytes.NewReader(emptyComponent), DefaultFeatures())
	var payloads []Payload
	for {
		p, err := vp.Next()
		if err == io.EOF {
			break
		}
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

func TestTopLevelComponentTypeIsExposed(t *testing.T) {
	src := `(component
  (type (func))
  (import "hostfn" (func (type 0)))
  (export "f" (func 0))
)`
	bin := wat2wasm(t, src)
	vp := NewValidatingParser(bytes.NewReader(bin), DefaultFeatures())
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	ct := vp.TopLevelComponentType()
	if ct == nil {
		t.Fatal("expected top-level ComponentType after parse")
	}
	internal := ct.arena.ComponentTypes[ct.id]
	if _, has := internal.Imports["hostfn"]; !has {
		t.Fatalf("expected import hostfn, got %v", internal.Imports)
	}
	if _, has := internal.Exports["f"]; !has {
		t.Fatalf("expected export f, got %v", internal.Exports)
	}
}

// TestValidatorResourceAlternateNameOrderIndependent verifies that
// bracket-prefixed func names (`[constructor]`, `[method]`, `[static]`)
// match any exported name of a resource, regardless of which export
// came first. Previously the validator latched onto whichever name was
// registered last, rejecting bracket names that referenced an earlier
// alias.
func TestValidatorResourceAlternateNameOrderIndependent(t *testing.T) {
	cases := []struct {
		name string
		wat  string
	}{
		{
			name: "method-after-alternate-export",
			wat: `(component
  (type $r (resource (rep i32)))
  (export $r-out "r" (type $r))
  (export "r-alt" (type $r-out))
  (core module $m (func (export "f") (param i32 i32)))
  (core instance $mi (instantiate $m))
  (func $simple (param "self" (borrow $r-out)) (param "x" u32)
    (canon lift (core func $mi "f")))
  (export "[method]r.simple" (func $simple)))`,
		},
		{
			name: "method-using-alternate-name",
			wat: `(component
  (type $r (resource (rep i32)))
  (export $r-out "r" (type $r))
  (export "r-alt" (type $r-out))
  (core module $m (func (export "f") (param i32 i32)))
  (core instance $mi (instantiate $m))
  (func $simple (param "self" (borrow $r-out)) (param "x" u32)
    (canon lift (core func $mi "f")))
  (export "[method]r-alt.simple" (func $simple)))`,
		},
		{
			name: "constructor-after-alternate-export",
			wat: `(component
  (type $r (resource (rep i32)))
  (export $r-out "r" (type $r))
  (export "r-alt" (type $r-out))
  (core func $new (canon resource.new $r))
  (core module $m
    (import "" "new" (func $new (param i32) (result i32)))
    (func (export "ctor") (param i32) (result i32) (call $new (local.get 0))))
  (core instance $mi (instantiate $m
    (with "" (instance (export "new" (func $new))))))
  (func $ctor (param "x" u32) (result (own $r-out))
    (canon lift (core func $mi "ctor")))
  (export "[constructor]r" (func $ctor)))`,
		},
		{
			name: "static-after-alternate-export",
			wat: `(component
  (type $r (resource (rep i32)))
  (export $r-out "r" (type $r))
  (export "r-alt" (type $r-out))
  (core module $m (func (export "f") (result i32) (i32.const 0)))
  (core instance $mi (instantiate $m))
  (func $s (result u32) (canon lift (core func $mi "f")))
  (export "[static]r.count" (func $s)))`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := wat2wasm(t, tc.wat)
			vp := NewValidatingParser(bytes.NewReader(bin), AllFeatures())
			for {
				_, err := vp.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
			}
		})
	}
}
