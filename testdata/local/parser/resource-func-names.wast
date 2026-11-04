;; Local-only tests: a bracket-prefixed func name must match the name of the resource
;; reference its signature uses. A reference has one name, in the context where the
;; import or export created it, even when several refer to one resource.
;; Each expectation was checked against wasm-tools 1.254.0 and wasmtime 50.0.1.

;; through the export's own reference
(component
  (component
    (type $r (resource (rep i32)))
    (export $a "a" (type $r))
    (core module $m (func (export "f") (param i32)))
    (core instance $i (instantiate $m))
    (type $bb (borrow $a))
    (func $f (param "self" $bb) (canon lift (core func $i "f")))
    (export "[method]a.foo" (func $f))))

;; a resource exported twice: each reference has only its own name
(component
  (component
    (type $r (resource (rep i32)))
    (export $a "a" (type $r))
    (export $b "b" (type $r))
    (core module $m (func (export "f") (param i32)))
    (core instance $i (instantiate $m))
    (type $bb (borrow $b))
    (func $f (param "self" $bb) (canon lift (core func $i "f")))
    (export "[method]b.foo" (func $f))))

(assert_invalid
  (component
    (component
      (type $r (resource (rep i32)))
      (export $a "a" (type $r))
      (export $b "b" (type $r))
      (core module $m (func (export "f") (param i32)))
      (core instance $i (instantiate $m))
      (type $bb (borrow $a))
      (func $f (param "self" $bb) (canon lift (core func $i "f")))
      (export "[method]b.foo" (func $f))))
  "function does not match expected resource name `a`")

;; an import's reference has no name among the exports
(assert_invalid
  (component
    (component
      (import "b" (type $T (sub resource)))
      (type $b (borrow $T))
      (type $f (func (param "self" $b)))
      (import "f" (func $f (type $f)))
      (export "c" (type $T))
      (export "[method]c.foo" (func $f))))
  "resource used in function does not have a name in this context")

;; ascribing the export's own reference gives it one
(component
  (component
    (import "c" (type $T (sub resource)))
    (type $b (borrow $T))
    (type $f (func (param "self" $b)))
    (import "f" (func $f (type $f)))
    (export $e "c" (type $T))
    (type $b2 (borrow $e))
    (type $f2 (func (param "self" $b2)))
    (export "[method]c.foo" (func $f) (func (type $f2)))))

;; in an import, the import's own reference
(component
  (component
    (import "c" (type $T (sub resource)))
    (type $b (borrow $T))
    (import "[method]c.foo" (func (param "self" $b)))))

;; an import can't attach to an export's name
(assert_invalid
  (component
    (component
      (type $r (resource (rep i32)))
      (export $e "c" (type $r))
      (type $b (borrow $e))
      (import "[method]c.foo" (func (param "self" $b)))))
  "")

(assert_invalid
  (component
    (component
      (type $r (resource (rep i32)))
      (export $e "c" (type $r))
      (import "[static]c.foo" (func))))
  "static resource name is not known in this context")

;; a constructor, through the right reference and the wrong one
(component
  (component
    (import "a" (type $T (sub resource)))
    (type $o (own $T))
    (import "[constructor]a" (func (result $o)))))

(assert_invalid
  (component
    (component
      (import "a" (type $T (sub resource)))
      (import "b" (type $U (eq $T)))
      (type $o (own $U))
      (import "[constructor]a" (func (result $o)))))
  "function does not match expected resource name `b`")

;; a static func only needs its resource's name known
(component
  (component
    (import "a" (type $T (sub resource)))
    (import "[static]a.make" (func))))

;; a re-export is a new reference with its own name; the func uses the first
(component
  (type $r (resource (rep i32)))
  (export $r-out "r" (type $r))
  (export "r-alt" (type $r-out))
  (core module $m (func (export "f") (param i32 i32)))
  (core instance $mi (instantiate $m))
  (func $simple (param "self" (borrow $r-out)) (param "x" u32)
    (canon lift (core func $mi "f")))
  (export "[method]r.simple" (func $simple)))

(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (export $r-out "r" (type $r))
    (export "r-alt" (type $r-out))
    (core module $m (func (export "f") (param i32 i32)))
    (core instance $mi (instantiate $m))
    (func $simple (param "self" (borrow $r-out)) (param "x" u32)
      (canon lift (core func $mi "f")))
    (export "[method]r-alt.simple" (func $simple)))
  "function does not match expected resource name `r`")
