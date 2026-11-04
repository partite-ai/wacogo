;; Local-only tests: each import of an instance is a separate instance with its own
;; resources, even when two share an instance type, and in a component type just as
;; in a component definition.
;; Each expectation was checked against wasm-tools 1.254.0 and wasmtime 50.0.1, and
;; they agree except where noted.
;; Everything is nested so that nothing at the root imports a component.

;; --- A component type's two imports take two different instances.

(component
  (component $wrap
    (type $it (instance (export "r" (type (sub resource)))))
    (import "c" (component $c
      (alias outer 1 $it (type $t))
      (import "a" (instance (type $t)))
      (import "b" (instance (type $t)))))
    (import "i" (instance $i (type $it)))
    (import "j" (instance $j (type $it)))
    (instance (instantiate $c (with "a" (instance $i)) (with "b" (instance $j))))))

;; or one instance for both
(component
  (component $wrap
    (type $it (instance (export "r" (type (sub resource)))))
    (import "c" (component $c
      (alias outer 1 $it (type $t))
      (import "a" (instance (type $t)))
      (import "b" (instance (type $t)))))
    (import "i" (instance $i (type $it)))
    (instance (instantiate $c (with "a" (instance $i)) (with "b" (instance $i))))))

;; and resources in nested instances are separate too
(component
  (component $wrap
    (type $it (instance (export "n" (instance (export "r" (type (sub resource)))))))
    (import "c" (component $c
      (alias outer 1 $it (type $t))
      (import "a" (instance (type $t)))
      (import "b" (instance (type $t)))))
    (import "i" (instance $i (type $it)))
    (import "j" (instance $j (type $it)))
    (instance (instantiate $c (with "a" (instance $i)) (with "b" (instance $j))))))

;; --- So a component that needs them to be one resource doesn't fit the type.

(assert_invalid
  (component
    (component $wrap
      (type $it (instance (export "r" (type (sub resource)))))
      (component $d
        (import "a" (instance $a (export "r" (type (sub resource)))))
        (alias export $a "r" (type $ar))
        (import "b" (instance (export "r" (type (eq $ar))))))
      (component $k
        (alias outer 1 $it (type $t))
        (import "x" (component
          (alias outer 1 $t (type $t2))
          (import "a" (instance (type $t2)))
          (import "b" (instance (type $t2))))))
      (instance (instantiate $k (with "x" (component $d))))))
  "resource types are not the same")

;; --- A func in a component type can tell them apart.

;; a component using the resource the type says fits
(component
  (component $wrap
    (type $it (instance (export "r" (type (sub resource)))))
    (component $d
      (import "a" (instance $a (export "r" (type (sub resource)))))
      (import "b" (instance $b (export "r" (type (sub resource)))))
      (alias export $b "r" (type $br))
      (type $o (own $br))
      (import "f" (func (param "x" $o))))
    (component $k
      (alias outer 1 $it (type $t))
      (import "x" (component
        (alias outer 1 $t (type $t2))
        (import "a" (instance (type $t2)))
        (import "b" (instance $b (type $t2)))
        (alias export $b "r" (type $br))
        (type $o (own $br))
        (import "f" (func (param "x" $o))))))
    (instance (instantiate $k (with "x" (component $d))))))

;; a component using the other one doesn't
(assert_invalid
  (component
    (component $wrap
      (type $it (instance (export "r" (type (sub resource)))))
      (component $d
        (import "a" (instance $a (export "r" (type (sub resource)))))
        (import "b" (instance $b (export "r" (type (sub resource)))))
        (alias export $a "r" (type $ar))
        (type $o (own $ar))
        (import "f" (func (param "x" $o))))
      (component $k
        (alias outer 1 $it (type $t))
        (import "x" (component
          (alias outer 1 $t (type $t2))
          (import "a" (instance (type $t2)))
          (import "b" (instance $b (type $t2)))
          (alias export $b "r" (type $br))
          (type $o (own $br))
          (import "f" (func (param "x" $o))))))
      (instance (instantiate $k (with "x" (component $d))))))
  "resource types are not the same")

;; --- A resource from outside the instance type stays shared by both imports.
;; wasmtime accepts this. wasm-tools 1.254.0 panics on it (assertion in
;; component_types.rs), so it gives no verdict.

(component
  (component $wrap
    (import "s" (type $s (sub resource)))
    (type $it (instance (export "r" (type (eq $s)))))
    (import "c" (component $c
      (alias outer 1 $it (type $t))
      (import "a" (instance (type $t)))
      (import "b" (instance (type $t)))))
    (import "i" (instance $i (type $it)))
    (import "j" (instance $j (type $it)))
    (instance (instantiate $c (with "a" (instance $i)) (with "b" (instance $j))))))
