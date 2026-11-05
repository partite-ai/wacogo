;; Local-only tests: a named primitive type compares by the primitive it is.
;; Each expectation was checked against wasm-tools 1.254.0 and wasmtime 50.0.1.

(assert_invalid
  (component
    (component
      (type $a u8)
      (export $ea "a" (type $a))
      (component $c (type $x s8) (import "t" (type (eq $x))))
      (instance (instantiate $c (with "t" (type $ea))))))
  "expected primitive `s8` found primitive `u8`")

(assert_invalid
  (component
    (component
      (type $a u8)
      (export $ea "a" (type $a))
      (instance $s (export "t" (type $ea)))
      (component $c (type $x s8) (import "i" (instance (export "t" (type (eq $x))))))
      (instance (instantiate $c (with "i" (instance $s))))))
  "expected primitive `s8` found primitive `u8`")

;; through an imported component's export
(assert_invalid
  (component
    (component $c
      (type (component (type u32) (export "t" (type (eq 0)))))
      (import "x" (component $c (type 0))))
    (component $x
      (type $t bool)
      (export "t" (type $t)))
    (instance (instantiate $c (with "x" (component $x)))))
  "expected primitive `u32` found primitive `bool`")

;; the same primitive matches
(component
  (component
    (type $a u8)
    (export $ea "a" (type $a))
    (component $c (type $x u8) (import "t" (type (eq $x))))
    (instance (instantiate $c (with "t" (type $ea))))))
