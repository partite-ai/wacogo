package core

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo/wasmtools"
)

// linkProvider is one instantiation in a linkCase. with maps its imports to
// paths, as in linkCase.links.
type linkProvider struct {
	label string
	wat   string
	with  map[string]string
}

// linkCase links instances, named by paths such as "p.n.m" (the export n.m of
// the provider labelled p), directly to a consumer's imports from Go.
type linkCase struct {
	name      string
	providers []linkProvider
	consumer  string
	links     map[string]string
	// funcLinks and typeLinks link a func or type export, named by a path whose
	// last element is the export.
	funcLinks map[string]string
	typeLinks map[string]string
	wantErr   string // empty if the link is valid

	// uncomposable marks a link that can't be written inside one component,
	// because an ascription hides the path it passes.
	uncomposable bool
}

const eqResourceConsumer = `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (import "b" (instance (export "r" (type (eq $r))))))`

const resourceAndChild = `(component
  (type $r (resource (rep i32)))
  (instance $n (export "r" (type $r)))
  (export "r" (type $r))
  (export "n" (instance $n)))`

const lifted = `(core module $m (func (export "f") (param i32)))
  (core instance $i (instantiate $m))`

// nestedSubInstances returns a component that re-exports a resource through
// depth levels of sub-instances, each of which adds a hop to its identity.
func nestedSubInstances(depth int) string {
	c := `(component (type $r (resource (rep i32))) (export "r" (type $r)))`
	for i := 0; i < depth; i++ {
		c = fmt.Sprintf(`(component
  %s
  (instance $i (instantiate 0))
  (alias export $i "r" (type $r))
  (export "r" (type $r))
  (export "i" (instance $i)))`, c)
	}
	return c
}

// funcsWithResource exports a resource, a func naming it from the root, and
// another from an inline instance.
const funcsWithResource = `(component
  ` + lifted + `
  (type $r (resource (rep i32)))
  (export $rx "r" (type $r))
  (type $o (own $rx))
  (func $f (param "x" $o) (canon lift (core func $i "f")))
  (export "f" (func $f))
  (func $g (param "x" $o) (canon lift (core func $i "f")))
  (instance $n (export "g" (func $g)))
  (export "n" (instance $n)))`

const resourceFuncConsumer = `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type $o (own $r))
  (import "f" (func (param "x" $o))))`

const twoResourceFuncsConsumer = `(component
  (import "r" (type $r (sub resource)))
  (type $o (own $r))
  (import "f" (func (param "x" $o)))
  (import "g" (func (param "x" $o))))`

var linkCases = []linkCase{
	{
		name: "func from a nested instance mismatch",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n)))`}},
		consumer:  `(component (import "f" (func (param "x" string))))`,
		funcLinks: map[string]string{"f": "p.n.f"},
		wantErr:   "expected primitive",
	},
	{
		name: "func from a nested instance match",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n)))`}},
		consumer:  `(component (import "f" (func (param "x" s32))))`,
		funcLinks: map[string]string{"f": "p.n.f"},
	},
	{
		name:      "root func with its instance's resource",
		providers: []linkProvider{{label: "p", wat: funcsWithResource}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p"},
		funcLinks: map[string]string{"f": "p.f"},
	},
	{
		name:      "root func with another instance's resource",
		providers: []linkProvider{{label: "p1", wat: funcsWithResource}, {label: "p2", wat: funcsWithResource}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p1"},
		funcLinks: map[string]string{"f": "p2.f"},
		wantErr:   "mismatched resource types",
	},
	{
		name:      "nested func with its instance's resource",
		providers: []linkProvider{{label: "p", wat: funcsWithResource}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p"},
		funcLinks: map[string]string{"f": "p.n.g"},
	},
	{
		name:      "nested func with another instance's resource",
		providers: []linkProvider{{label: "p1", wat: funcsWithResource}, {label: "p2", wat: funcsWithResource}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p1"},
		funcLinks: map[string]string{"f": "p2.n.g"},
		wantErr:   "mismatched resource types",
	},
	{
		name:      "funcs from one instantiation share a resource",
		providers: []linkProvider{{label: "p", wat: funcsWithResource}},
		consumer:  twoResourceFuncsConsumer,
		typeLinks: map[string]string{"r": "p.r"},
		funcLinks: map[string]string{"f": "p.f", "g": "p.n.g"},
	},
	{
		name:      "funcs from two instantiations differ",
		providers: []linkProvider{{label: "p1", wat: funcsWithResource}, {label: "p2", wat: funcsWithResource}},
		consumer:  twoResourceFuncsConsumer,
		typeLinks: map[string]string{"r": "p1.r"},
		funcLinks: map[string]string{"f": "p1.f", "g": "p2.f"},
		wantErr:   "mismatched resource types",
	},
	{
		name: "func from a sub-instance with the parent's view of its resource",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $inner
    ` + lifted + `
    (type $r (resource (rep i32)))
    (export $rx "r" (type $r))
    (type $o (own $rx))
    (func $f (param "x" $o) (canon lift (core func $i "f")))
    (export "f" (func $f)))
  (instance $s (instantiate $inner))
  (alias export $s "r" (type $r))
  (export "r" (type $r))
  (export "s" (instance $s)))`}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p"},
		funcLinks: map[string]string{"f": "p.s.f"},
	},
	{
		name: "func re-exported by the parent of the sub-instance that lifted it",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $inner
    ` + lifted + `
    (type $r (resource (rep i32)))
    (export $rx "r" (type $r))
    (type $o (own $rx))
    (func $f (param "x" $o) (canon lift (core func $i "f")))
    (export "f" (func $f)))
  (instance $s (instantiate $inner))
  (alias export $s "r" (type $r))
  (alias export $s "f" (func $sf))
  (export $rx "r" (type $r))
  (type $o (own $rx))
  (export "g" (func $sf) (func (param "x" $o))))`}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p"},
		funcLinks: map[string]string{"f": "p.g"},
	},
	{
		name: "func from an imported sub-component",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $impl
    ` + lifted + `
    (type $r (resource (rep i32)))
    (export $rx "r" (type $r))
    (type $o (own $rx))
    (func $f (param "x" $o) (canon lift (core func $i "f")))
    (export "f" (func $f)))
  (component $user
    (import "c" (component $c
      (export "r" (type $r (sub resource)))
      (type $o (own $r))
      (export "f" (func (param "x" $o)))))
    (instance $made (instantiate $c))
    (export "made" (instance $made)))
  (instance $u (instantiate $user (with "c" (component $impl))))
  (export "u" (instance $u)))`}},
		consumer:  resourceFuncConsumer,
		links:     map[string]string{"a": "p.u.made"},
		funcLinks: map[string]string{"f": "p.u.made.f"},
	},
	{
		name: "nested instance mismatch",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n)))`}},
		consumer: `(component (import "n" (instance (export "f" (func (param "x" string))))))`,
		links:    map[string]string{"n": "p.n"},
		wantErr:  "expected primitive",
	},
	{
		name: "nested instance match",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n)))`}},
		consumer: `(component (import "n" (instance (export "f" (func (param "x" s32))))))`,
		links:    map[string]string{"n": "p.n"},
	},
	{
		name:      "root and child give one resource",
		providers: []linkProvider{{label: "p", wat: resourceAndChild}},
		consumer:  eqResourceConsumer,
		links:     map[string]string{"a": "p", "b": "p.n"},
	},
	{
		name:      "children of two instantiations differ",
		providers: []linkProvider{{label: "p1", wat: resourceAndChild}, {label: "p2", wat: resourceAndChild}},
		consumer:  eqResourceConsumer,
		links:     map[string]string{"a": "p1.n", "b": "p2.n"},
		wantErr:   "mismatched resource types",
	},
	{
		name:      "root and child of two instantiations differ",
		providers: []linkProvider{{label: "p1", wat: resourceAndChild}, {label: "p2", wat: resourceAndChild}},
		consumer:  eqResourceConsumer,
		links:     map[string]string{"a": "p1", "b": "p2.n"},
		wantErr:   "mismatched resource types",
	},
	{
		name: "resource named by a func in the child",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (type $r (resource (rep i32)))
  (export $rx "r" (type $r))
  (type $o (own $rx))
  (func $f (param "x" $o) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n)))`}},
		consumer: `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type $o (own $r))
  (import "b" (instance (export "f" (func (param "x" $o))))))`,
		links: map[string]string{"a": "p", "b": "p.n"},
	},
	{
		name: "sub-instance and its child give one resource",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $inner
    (type $r (resource (rep i32)))
    (instance $d (export "r" (type $r)))
    (export "r" (type $r))
    (export "d" (instance $d)))
  (instance $i (instantiate $inner))
  (export "i" (instance $i)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.i", "b": "p.i.d"},
	},
	{
		name: "two instantiations of a sub-component differ",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $inner
    (type $r (resource (rep i32)))
    (instance $d (export "r" (type $r)))
    (export "r" (type $r))
    (export "d" (instance $d)))
  (instance $i1 (instantiate $inner))
  (instance $i2 (instantiate $inner))
  (export "i1" (instance $i1))
  (export "i2" (instance $i2)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.i1.d", "b": "p.i2.d"},
		wantErr:  "mismatched resource types",
	},
	{
		name: "resource passed down as a type argument",
		providers: []linkProvider{{label: "p", wat: `(component
  (type $r (resource (rep i32)))
  (component $k (import "r" (type $ir (sub resource))) (export "r" (type $ir)))
  (instance $ki (instantiate $k (with "r" (type $r))))
  (export "r" (type $r))
  (export "ki" (instance $ki)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p", "b": "p.ki"},
	},
	{
		name: "one type argument given to two sub-components",
		providers: []linkProvider{{label: "p", wat: `(component
  (type $r (resource (rep i32)))
  (component $k (import "r" (type $ir (sub resource))) (export "r" (type $ir)))
  (instance $k1 (instantiate $k (with "r" (type $r))))
  (instance $k2 (instantiate $k (with "r" (type $r))))
  (export "k1" (instance $k1))
  (export "k2" (instance $k2)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.k1", "b": "p.k2"},
	},
	{
		name: "root re-exports a resource created by its sub-instance",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $inner (type $r (resource (rep i32))) (export "r" (type $r)))
  (instance $i (instantiate $inner))
  (alias export $i "r" (type $r))
  (export "r" (type $r))
  (export "i" (instance $i)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p", "b": "p.i"},
	},
	{
		name: "resource created by one sub-instance and forwarded by another",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $maker (type $r (resource (rep i32))) (export "r" (type $r)))
  (component $fwd (import "r" (type $ir (sub resource))) (export "r" (type $ir)))
  (instance $m (instantiate $maker))
  (alias export $m "r" (type $r))
  (instance $f (instantiate $fwd (with "r" (type $r))))
  (export "m" (instance $m))
  (export "f" (instance $f)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.m", "b": "p.f"},
	},
	{
		// Written as one component this is invalid, because the declared type of
		// $c doesn't say that it forwards r. The instances are checked by their
		// actual types, and r is one resource.
		name: "resource forwarded by an imported sub-component declared as creating it",
		providers: []linkProvider{{label: "p", wat: `(component
  (type $r (resource (rep i32)))
  (component $fwd (import "r" (type $ir (sub resource))) (export "r" (type $ir)))
  (component $user
    (import "r" (type $ur (sub resource)))
    (import "c" (component $c
      (import "r" (type (sub resource)))
      (export "r" (type (sub resource)))))
    (instance $i (instantiate $c (with "r" (type $ur))))
    (export "i" (instance $i)))
  (instance $u (instantiate $user (with "r" (type $r)) (with "c" (component $fwd))))
  (export "r" (type $r))
  (export "u" (instance $u)))`}},
		consumer:     eqResourceConsumer,
		links:        map[string]string{"a": "p", "b": "p.u.i"},
		uncomposable: true,
	},
	{
		name: "component reached through an outer alias",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $impl
    (type $r (resource (rep i32)))
    (instance $n (export "r" (type $r)))
    (export "r" (type $r))
    (export "n" (instance $n)))
  (component $user
    (alias outer 1 0 (component $c))
    (instance $i (instantiate $c))
    (alias export $i "r" (type $r))
    (export "r" (type $r))
    (export "i" (instance $i)))
  (instance $u (instantiate $user))
  (export "u" (instance $u)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.u", "b": "p.u.i.n"},
	},
	{
		// $runner has a decoy at the index $user's outer alias names, which only
		// the frame $user was defined in resolves correctly.
		name: "outer alias resolves where its component was defined",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $impl
    (type $r (resource (rep i32)))
    (instance $n (export "r" (type $r)))
    (export "r" (type $r))
    (export "n" (instance $n)))
  (component $user
    (alias outer 1 0 (component $c))
    (instance $i (instantiate $c))
    (alias export $i "r" (type $r))
    (export "r" (type $r))
    (export "i" (instance $i)))
  (component $runner
    (component $decoy)
    (import "c" (component $c
      (export "r" (type $r (sub resource)))
      (export "i" (instance (export "r" (type (eq $r)))))))
    (instance $made (instantiate $c))
    (export "made" (instance $made)))
  (instance $run (instantiate $runner (with "c" (component $user))))
  (export "run" (instance $run)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.run.made.i", "b": "p.run.made"},
	},
	{
		name: "component passed through an inline instance",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $impl
    (type $r (resource (rep i32)))
    (instance $extra (export "r" (type $r)))
    (export "r" (type $r))
    (export "extra" (instance $extra)))
  (instance $n (export "c" (component $impl)))
  (component $user
    (import "i" (instance $i (export "c" (component (export "r" (type (sub resource)))))))
    (alias export $i "c" (component $c))
    (instance $made (instantiate $c))
    (alias export $made "r" (type $r))
    (export "r" (type $r))
    (export "made" (instance $made)))
  (instance $u (instantiate $user (with "i" (instance $n))))
  (export "u" (instance $u)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.u", "b": "p.u.made"},
	},
	{
		name: "exports between instances keep each instance's own type",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (func $g (param "x" u32) (canon lift (core func $i "f")))
  (instance $a (export "f" (func $f)))
  (export "a1" (instance $a))
  (instance $b (export "f" (func $g)))
  (export "b1" (instance $b)))`}},
		consumer: `(component (import "n" (instance (export "f" (func (param "x" s32))))))`,
		links:    map[string]string{"n": "p.b1"},
		wantErr:  "expected primitive",
	},
	{
		name: "instance argument re-exported by a sub-component",
		providers: []linkProvider{{label: "p", wat: `(component
  (type $r (resource (rep i32)))
  (instance $n (export "r" (type $r)))
  (component $k
    (import "i" (instance $x (export "r" (type (sub resource)))))
    (export "o" (instance $x)))
  (instance $ki (instantiate $k (with "i" (instance $n))))
  (export "r" (type $r))
  (export "ki" (instance $ki)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p", "b": "p.ki.o"},
	},
	{
		name: "imported sub-component wider than declared",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $impl
    (type $r (resource (rep i32)))
    (instance $extra (export "r" (type $r)))
    (export "r" (type $r))
    (export "extra" (instance $extra)))
  (component $user
    (import "c" (component $ci (export "r" (type (sub resource)))))
    (instance $i (instantiate $ci))
    (alias export $i "r" (type $r))
    (export "r" (type $r))
    (export "i" (instance $i)))
  (instance $u (instantiate $user (with "c" (component $impl))))
  (export "u" (instance $u)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.u", "b": "p.u.i"},
	},
	{
		name: "two instances of an imported sub-component differ",
		providers: []linkProvider{{label: "p", wat: `(component
  (component $impl (type $r (resource (rep i32))) (export "r" (type $r)))
  (component $user
    (import "c" (component $ci (export "r" (type (sub resource)))))
    (instance $i1 (instantiate $ci))
    (instance $i2 (instantiate $ci))
    (export "i1" (instance $i1))
    (export "i2" (instance $i2)))
  (instance $u (instantiate $user (with "c" (component $impl))))
  (export "u" (instance $u)))`}},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "p.u.i1", "b": "p.u.i2"},
		wantErr:  "mismatched resource types",
	},
	{
		name:      "long resource chain",
		providers: []linkProvider{{label: "p", wat: nestedSubInstances(12)}},
		consumer:  eqResourceConsumer,
		links:     map[string]string{"a": "p", "b": "p" + strings.Repeat(".i", 12)},
	},
	{
		name: "supplied instance re-exported as is",
		providers: []linkProvider{
			{label: "l", wat: resourceAndChild},
			{label: "p", wat: `(component
  (import "s" (instance $s (export "r" (type (sub resource)))))
  (export "s" (instance $s)))`, with: map[string]string{"s": "l"}},
		},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "l", "b": "p.s"},
	},
	{
		name: "imported resource in an inline instance",
		providers: []linkProvider{
			{label: "l", wat: resourceAndChild},
			{label: "p", wat: `(component
  (import "s" (instance $s (export "r" (type (sub resource)))))
  (alias export $s "r" (type $r))
  (instance $n (export "r" (type $r)))
  (export "n" (instance $n)))`, with: map[string]string{"s": "l.n"}},
		},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "l", "b": "p.n"},
	},
	{
		name: "imported resource in an inline instance from another lender",
		providers: []linkProvider{
			{label: "l1", wat: resourceAndChild},
			{label: "l2", wat: resourceAndChild},
			{label: "p", wat: `(component
  (import "s" (instance $s (export "r" (type (sub resource)))))
  (alias export $s "r" (type $r))
  (instance $n (export "r" (type $r)))
  (export "n" (instance $n)))`, with: map[string]string{"s": "l1"}},
		},
		consumer: eqResourceConsumer,
		links:    map[string]string{"a": "l2", "b": "p.n"},
		wantErr:  "mismatched resource types",
	},
	{
		name: "imported resource hidden from the root by an ascription",
		providers: []linkProvider{
			{label: "l", wat: resourceAndChild},
			{label: "p", wat: `(component
  (import "s" (instance $s (export "r" (type (sub resource)))))
  (alias export $s "r" (type $r))
  (instance $n (export "r" (type $r)))
  (export "n" (instance $n) (instance)))`, with: map[string]string{"s": "l"}},
		},
		consumer:     eqResourceConsumer,
		links:        map[string]string{"a": "l", "b": "p.n"},
		uncomposable: true,
	},
	{
		name: "child narrowed by an ascription is checked by its actual type",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n) (instance)))`}},
		consumer:     `(component (import "n" (instance (export "f" (func (param "x" s32))))))`,
		links:        map[string]string{"n": "p.n"},
		uncomposable: true,
	},
	{
		name: "parent's type includes the ascription narrowing its child",
		providers: []linkProvider{{label: "p", wat: `(component
  ` + lifted + `
  (func $f (param "x" s32) (canon lift (core func $i "f")))
  (instance $n (export "f" (func $f)))
  (export "n" (instance $n) (instance)))`}},
		consumer: `(component
  (import "p" (instance (export "n" (instance (export "f" (func (param "x" s32))))))))`,
		links:   map[string]string{"p": "p"},
		wantErr: "was not found",
	},
}

// composeLink writes c as a single component, for the reference tools to judge.
func composeLink(c linkCase) string {
	var b strings.Builder
	b.WriteString("(component\n")
	aliased := map[string]bool{}
	// ref declares the aliases a path needs and returns its instance's name.
	ref := func(path string) string {
		parts := strings.Split(path, ".")
		name := "$" + parts[0]
		for _, part := range parts[1:] {
			next := name + "_" + part
			if !aliased[next] {
				aliased[next] = true
				fmt.Fprintf(&b, "(alias export %s %q (instance %s))\n", name, part, next)
			}
			name = next
		}
		return name
	}
	instantiate := func(id, wat string, with map[string]string) string {
		fmt.Fprintf(&b, "(component %s %s)\n", id, strings.TrimPrefix(wat, "(component"))
		var args strings.Builder
		for imp, path := range with {
			fmt.Fprintf(&args, " (with %q (instance %s))", imp, ref(path))
		}
		return args.String()
	}
	for _, p := range c.providers {
		args := instantiate("$c_"+p.label, p.wat[:len(p.wat)-1], p.with)
		fmt.Fprintf(&b, "(instance $%s (instantiate $c_%s%s))\n", p.label, p.label, args)
	}
	args := instantiate("$consumer", c.consumer[:len(c.consumer)-1], c.links)
	item := func(kind string, links map[string]string) {
		for imp, path := range links {
			split := strings.LastIndex(path, ".")
			next := fmt.Sprintf("$%s_%s", kind, imp)
			fmt.Fprintf(&b, "(alias export %s %q (%s %s))\n", ref(path[:split]), path[split+1:], kind, next)
			args += fmt.Sprintf(" (with %q (%s %s))", imp, kind, next)
		}
	}
	item("func", c.funcLinks)
	item("type", c.typeLinks)
	fmt.Fprintf(&b, "(instance (instantiate $consumer%s)))\n", args)
	return b.String()
}

// parseLinkWat converts WAT to a component binary.
func parseLinkWat(t *testing.T, wat string) []byte {
	t.Helper()
	tool, err := wasmtools.Default(context.Background())
	if err != nil {
		t.Fatalf("wasmtools.Default: %v", err)
	}
	bin, err := tool.Parse(context.Background(), []byte(wat))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return bin
}

// checkComposedVerdict confirms wantErr against the validator's verdict on c
// written as a single component, where every link is checked by the spec's rules.
func checkComposedVerdict(t *testing.T, engine *Engine, c linkCase) {
	t.Helper()
	_, err := engine.LoadComponent(context.Background(), bytes.NewReader(parseLinkWat(t, composeLink(c))))
	if wantValid := c.wantErr == ""; (err == nil) != wantValid {
		t.Fatalf("composed form: want valid=%v, got: %v", wantValid, err)
	}
}

func resolveLinkPath(t *testing.T, insts map[string]*ComponentInstance, path string) *ComponentInstance {
	t.Helper()
	parts := strings.Split(path, ".")
	inst := insts[parts[0]]
	for _, part := range parts[1:] {
		if inst = inst.ExportedInstance(part); inst == nil {
			t.Fatalf("path %q: no instance at %q", path, part)
		}
	}
	return inst
}

// assertInstanceTypesAligned checks that each instance in a tree carries the type
// wasmparser evaluated for it, which holds only if the runtime's instance index
// space lines up with the validator's.
func assertInstanceTypesAligned(t *testing.T, path string, inst *ComponentInstance) {
	t.Helper()
	for name, exp := range inst.exports {
		childPath := path + "." + name
		if exp.kind == SortFunc && exp.funcVal.ParserFunctionType() == nil {
			t.Errorf("%s: func has no type", childPath)
		}
		if exp.kind != SortInstance {
			continue
		}
		want := inst.wpInstance.ExportedInstance(name)
		if want == nil {
			t.Errorf("%s: no type evaluated", childPath)
		}
		if exp.instance.wpInstance != want {
			t.Errorf("%s: instance holds type %p, evaluated %p", childPath, exp.instance.wpInstance, want)
		}
		assertInstanceTypesAligned(t, childPath, exp.instance)
	}
}

func TestInstanceLinkTypes(t *testing.T) {
	for _, c := range linkCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			engine := NewEngine(ctx)
			defer engine.Close(ctx)
			if !c.uncomposable {
				checkComposedVerdict(t, engine, c)
			}
			load := func(wat string) *Component {
				comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, wat)))
				if err != nil {
					t.Fatalf("load: %v", err)
				}
				return comp
			}
			opts := func(insts map[string]*ComponentInstance, links map[string]string) []InstantiateOption {
				var out []InstantiateOption
				for name, path := range links {
					out = append(out, WithInstanceImport(name, resolveLinkPath(t, insts, path)))
				}
				return out
			}

			parent := func(insts map[string]*ComponentInstance, path string) (*ComponentInstance, string) {
				split := strings.LastIndex(path, ".")
				return resolveLinkPath(t, insts, path[:split]), path[split+1:]
			}
			insts := map[string]*ComponentInstance{}
			for _, p := range c.providers {
				inst, err := load(p.wat).Instantiate(ctx, opts(insts, p.with)...)
				if err != nil {
					t.Fatalf("instantiate %s: %v", p.label, err)
				}
				insts[p.label] = inst
				assertInstanceTypesAligned(t, p.label, inst)
			}

			consumerOpts := opts(insts, c.links)
			for name, path := range c.funcLinks {
				inst, export := parent(insts, path)
				fn := inst.ExportedFunc(export)
				if fn == nil {
					t.Fatalf("path %q: no func", path)
				}
				consumerOpts = append(consumerOpts, WithFuncImport(name, fn))
			}
			for name, path := range c.typeLinks {
				inst, export := parent(insts, path)
				consumerOpts = append(consumerOpts, WithTypeImport(name, inst.ExportedType(export)))
			}
			_, err := load(c.consumer).Instantiate(ctx, consumerOpts...)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("want link accepted, got: %v", err)
			case c.wantErr != "" && err == nil:
				t.Fatalf("want link rejected with %q, was accepted", c.wantErr)
			case c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr):
				t.Fatalf("want error containing %q, got: %v", c.wantErr, err)
			}
		})
	}
}

// An omitted runtime-empty instance has a type too: it needs nothing at runtime,
// so it can't satisfy an import that does.
func TestInstanceLinkTypes_RuntimeEmptyPlaceholder(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	prov, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (import "e" (instance $e))
  (export "e" (instance $e)))`)))
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	p, err := prov.Instantiate(ctx)
	if err != nil {
		t.Fatalf("instantiate provider: %v", err)
	}
	cons, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (import "x" (instance (export "f" (func)))))`)))
	if err != nil {
		t.Fatalf("load consumer: %v", err)
	}
	_, err = cons.Instantiate(ctx, WithInstanceImport("x", p.ExportedInstance("e")))
	if err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("want link rejected with a missing export, got: %v", err)
	}
}

// An instance built without a type is skipped by the link-time check.
func TestInstanceLinkTypes_TypelessInstanceSkipped(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	typeless, err := NewInstance(engine, &InstanceSpec{})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	cons, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (import "x" (instance (export "f" (func)))))`)))
	if err != nil {
		t.Fatalf("load consumer: %v", err)
	}
	if _, err := cons.Instantiate(ctx, WithInstanceImport("x", typeless)); err != nil {
		t.Fatalf("want typeless instance accepted, got: %v", err)
	}
}

func TestRootExportExposingComponentRejected(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	for name, wat := range map[string]string{
		"component": `(component (component $c) (export "c" (component $c)))`,
		"inline instance": `(component
  (component $c)
  (instance $i (export "c" (component $c)))
  (export "i" (instance $i)))`,
		"nested inline instance": `(component
  (component $c)
  (instance $i (export "c" (component $c)))
  (instance $j (export "i" (instance $i)))
  (export "j" (instance $j)))`,
		"sub-instance": `(component
  (component $k (component $c) (export "c" (component $c)))
  (instance $i (instantiate $k))
  (export "i" (instance $i)))`,
		"hidden by an ascription": `(component
  (component $c)
  (instance $i (export "c" (component $c)))
  (export "i" (instance $i) (instance)))`,
		"hidden inside a sub-instance": `(component
  (component $k
    (component $c)
    (instance $i (export "c" (component $c)))
    (export "i" (instance $i) (instance)))
  (instance $s (instantiate $k))
  (export "s" (instance $s)))`,
		"imported and re-exported": `(component
  (import "i" (instance $i (export "c" (component))))
  (export "i" (instance $i)))`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, wat)))
			if err == nil || !strings.Contains(err.Error(), "exporting a component from the root component is not supported") {
				t.Fatalf("want root export rejected, got: %v", err)
			}
		})
	}

	// A component type export isn't a component, and an instance holding one is
	// fine as long as it stays inside.
	for _, wat := range []string{
		`(component (type $t (component)) (export "t" (type $t)))`,
		`(component (component $c) (instance $i (export "c" (component $c))))`,
	} {
		if _, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, wat))); err != nil {
			t.Fatalf("want %s accepted, got: %v", wat, err)
		}
	}

	// An inner component can still export one to its parent.
	comp := `(component
  (component $k (component $c) (export "c" (component $c)))
  (instance $i (instantiate $k))
  (alias export $i "c" (component $c))
  (instance (instantiate $c)))`
	if _, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, comp))); err != nil {
		t.Fatalf("want inner component export accepted, got: %v", err)
	}
}
