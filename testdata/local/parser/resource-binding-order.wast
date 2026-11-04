;; Local-only tests: what a component leaves open is known from its type and bound
;; before anything is compared, so it doesn't matter what names it first, and
;; a resource that isn't open compares by identity.
;; Each expectation was checked against wasm-tools 1.254.0 and wasmtime 50.0.1.

;; a component export naming its sibling resource, in either export order
(component
  (component $wrap
    (import "r" (type $r (sub resource)))
    (import "cc" (component $cc (alias outer 1 $r (type $r3)) (import "x" (type (eq $r3)))))
    (instance $i (export "r" (type $r)) (export "c" (component $cc)))
    (component $child
      (import "i" (instance
        (export "r" (type $r2 (sub resource)))
        (export "c" (component (alias outer 1 $r2 (type $r4)) (import "x" (type (eq $r4))))))))
    (instance (instantiate $child (with "i" (instance $i))))))

;; a resource bound by a later import than the one naming it isn't possible to
;; write, but an instance's nested resource is bound before its sibling's func
(component
  (component $wrap
    (import "i" (instance $i
      (export "n" (instance $n (export "r" (type $r (sub resource)))))
      (alias export $n "r" (type $r2))
      (type $o (own $r2))
      (export "o" (type (eq $o)))))
    (component $child
      (import "i" (instance
        (export "n" (instance $n (export "r" (type (sub resource)))))
        (alias export $n "r" (type $r2))
        (type $o (own $r2))
        (export "o" (type (eq $o))))))
    (instance (instantiate $child (with "i" (instance $i))))))

;; a resource that isn't open compares by identity
(assert_invalid
  (component
    (component $wrap
      (import "r" (type $r (sub resource)))
      (import "o" (type $o (sub resource)))
      (import "i" (instance $i (export "t" (type (eq $o)))))
      (import "c" (component $c
        (alias outer 1 $r (type $r2))
        (import "i" (instance (export "t" (type (eq $r2)))))))
      (instance (instantiate $c (with "i" (instance $i))))))
  "resource types are not the same")

;; the same resource passes
(component
  (component $wrap
    (import "r" (type $r (sub resource)))
    (import "i" (instance $i (export "t" (type (eq $r)))))
    (import "c" (component $c
      (alias outer 1 $r (type $r2))
      (import "i" (instance (export "t" (type (eq $r2)))))))
    (instance (instantiate $c (with "i" (instance $i))))))

;; an open resource is still held to what it was bound to
(assert_invalid
  (component
    (component $wrap
      (import "p" (type $p (sub resource)))
      (import "q" (type $q (sub resource)))
      (component $c
        (import "a" (type $a (sub resource)))
        (import "b" (type (eq $a))))
      (instance (instantiate $c (with "a" (type $p)) (with "b" (type $q))))))
  "resource types are not the same")
