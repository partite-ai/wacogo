;; Local-only tests: a component type is a subtype if it imports less and exports
;; more, and each type's own resources are bound rather than compared by identity.
;; Expectations were checked against wasm-tools 1.254.0 and wasmtime 50.0.1.


;; --- A subtype may import less, but not more. Instantiation and ascription agree.

(component (component
  (import "src" (component $c ))
  (export "out" (component $c) (component (import "a" (func))))
))

(component (component
  (import "src" (component $c ))
  (component $o (import "x" (component (import "a" (func)))))
  (instance (instantiate $o (with "x" (component $c))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "a" (func))))
  (export "out" (component $c) (component ))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (import "src" (component $c (import "a" (func))))
  (component $o (import "x" (component )))
  (instance (instantiate $o (with "x" (component $c))))
))
  "type mismatch for import")


;; --- Imports are contravariant.

(component (component
  (import "src" (component $c (import "a" (instance))))
  (export "out" (component $c) (component (import "a" (instance (export "f" (func))))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "a" (instance (export "f" (func))))))
  (export "out" (component $c) (component (import "a" (instance))))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (import "src" (component $c (import "a" (func (param "p" u32)))))
  (export "out" (component $c) (component (import "a" (func))))
))
  "ascribed type of export is not compatible")


;; --- ...and the direction flips again at each nested import, but not at an export.

(component (component
  (import "src" (component $c (import "y" (component (import "a" (func))))))
  (export "out" (component $c) (component (import "y" (component))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "y" (component))))
  (export "out" (component $c) (component (import "y" (component (import "a" (func))))))
))
  "ascribed type of export is not compatible")

(component (component
  (import "src" (component $c (import "y" (component (import "z" (component))))))
  (export "out" (component $c) (component (import "y" (component (import "z" (component (import "a" (func))))))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "y" (component (import "z" (component (import "a" (func))))))))
  (export "out" (component $c) (component (import "y" (component (import "z" (component))))))
))
  "ascribed type of export is not compatible")

(component (component
  (import "src" (component $c (export "y" (component))))
  (export "out" (component $c) (component (export "y" (component (import "a" (func))))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (export "y" (component (import "a" (func))))))
  (export "out" (component $c) (component (export "y" (component))))
))
  "ascribed type of export is not compatible")


;; --- A subtype may export more, but not less.

(component (component
  (import "src" (component $c (export "f" (func))))
  (export "out" (component $c) (component ))
))

(assert_invalid
  (component (component
  (import "src" (component $c ))
  (export "out" (component $c) (component (export "f" (func))))
))
  "ascribed type of export is not compatible")


;; --- A type's imported resources are bound to the supplied type's, by name.

(component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
  (export "out" (component $c) (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
))

(component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
  (component $o (import "x" (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r))))))
  (instance (instantiate $o (with "x" (component $c))))
))

(component (component
  (import "src" (component $c (import "r" (type (sub resource)))))
  (export "out" (component $c) (component (import "r" (type (sub resource)))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (import "s" (type (sub resource))) (export "e" (type (eq $r)))))
  (export "out" (component $c) (component (import "r" (type (sub resource))) (import "s" (type $s (sub resource))) (export "e" (type (eq $s)))))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (import "s" (type (sub resource))) (export "e" (type (eq $r)))))
  (component $o (import "x" (component (import "r" (type (sub resource))) (import "s" (type $s (sub resource))) (export "e" (type (eq $s))))))
  (instance (instantiate $o (with "x" (component $c))))
))
  "type mismatch for import")

(component (component
  (import "src" (component $c (import "i" (instance $i (export "r" (type (sub resource))))) (alias export $i "r" (type $r)) (export "e" (type (eq $r)))))
  (export "out" (component $c) (component (import "i" (instance $i (export "r" (type (sub resource))))) (alias export $i "r" (type $r)) (export "e" (type (eq $r)))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "i" (instance $i (export "r" (type (sub resource))) (export "s" (type (sub resource))))) (alias export $i "r" (type $r)) (export "e" (type (eq $r)))))
  (export "out" (component $c) (component (import "i" (instance $i (export "r" (type (sub resource))) (export "s" (type (sub resource))))) (alias export $i "s" (type $s)) (export "e" (type (eq $s)))))
))
  "ascribed type of export is not compatible")


;; --- A supertype may keep two imports apart or merge them only where the subtype allows.

(component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (import "s" (type $s (sub resource))) (type $o (own $s)) (export "e" (func (param "x" $o)))))
  (export "out" (component $c) (component (import "r" (type $r (sub resource))) (import "s" (type (eq $r))) (type $o (own $r)) (export "e" (func (param "x" $o)))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (import "s" (type (eq $r)))))
  (export "out" (component $c) (component (import "r" (type (sub resource))) (import "s" (type (sub resource)))))
))
  "ascribed type of export is not compatible")


;; --- A supertype's exported resources are bound to the subtype's, hiding what they are.

(component (component
  (import "src" (component $c (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
  (export "out" (component $c) (component (import "r" (type (sub resource))) (export "e" (type (sub resource)))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "r" (type (sub resource))) (export "e" (type (sub resource)))))
  (export "out" (component $c) (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (import "src" (component $c (import "r" (type (sub resource))) (export "e" (type (sub resource)))))
  (component $o (import "x" (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r))))))
  (instance (instantiate $o (with "x" (component $c))))
))
  "type mismatch for import")

(component (component
  (import "src" (component $c (export "p" (type $r (sub resource))) (export "q" (type (eq $r)))))
  (export "out" (component $c) (component (export "p" (type (sub resource))) (export "q" (type (sub resource)))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (export "p" (type (sub resource))) (export "q" (type (sub resource)))))
  (export "out" (component $c) (component (export "p" (type $r (sub resource))) (export "q" (type (eq $r)))))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (import "src" (component $c (export "i" (instance (export "p" (type (sub resource))) (export "q" (type (sub resource)))))))
  (export "out" (component $c) (component (export "i" (instance (export "p" (type $r (sub resource))) (export "q" (type (eq $r)))))))
))
  "ascribed type of export is not compatible")


;; --- Nested component types bind their own resources.

(component (component
  (import "src" (component $c (import "c" (component (import "r" (type (sub resource))) (export "e" (type (sub resource)))))))
  (export "out" (component $c) (component (import "c" (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))))
))

(assert_invalid
  (component (component
  (import "src" (component $c (import "c" (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))))
  (export "out" (component $c) (component (import "c" (component (import "r" (type (sub resource))) (export "e" (type (sub resource)))))))
))
  "ascribed type of export is not compatible")

(component (component
  (import "src" (component $c (import "x" (component (import "r" (type (sub resource))))) (import "y" (component (import "r" (type (sub resource)))))))
  (export "out" (component $c) (component (import "y" (component (import "r" (type (sub resource))))) (import "x" (component (import "r" (type (sub resource)))))))
))


;; --- Resources from outside a type compare by identity.

(component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (alias outer 1 $x (type $q)) (import "r" (type (eq $q)))))
  (export "out" (component $c) (component (alias outer 1 $x (type $q)) (import "r" (type (eq $q)))))
))

(assert_invalid
  (component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (alias outer 1 $x (type $q)) (import "r" (type (eq $q)))))
  (export "out" (component $c) (component (alias outer 1 $y (type $q)) (import "r" (type (eq $q)))))
))
  "ascribed type of export is not compatible")

(component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (import "r" (type (sub resource)))))
  (export "out" (component $c) (component (alias outer 1 $x (type $q)) (import "r" (type (eq $q)))))
))

(assert_invalid
  (component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (alias outer 1 $x (type $q)) (import "r" (type (eq $q)))))
  (export "out" (component $c) (component (import "r" (type (sub resource)))))
))
  "ascribed type of export is not compatible")

(component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (alias outer 1 $x (type $q)) (export "e" (type (eq $q)))))
  (export "out" (component $c) (component (export "e" (type (sub resource)))))
))

(assert_invalid
  (component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (export "e" (type (sub resource)))))
  (export "out" (component $c) (component (alias outer 1 $x (type $q)) (export "e" (type (eq $q)))))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (import "x" (type $x (sub resource))) (import "y" (type $y (sub resource)))
  (import "src" (component $c (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
  (export "out" (component $c) (component (alias outer 1 $x (type $q)) (import "r" (type (eq $q))) (alias outer 1 $y (type $p)) (export "e" (type (eq $p)))))
))
  "ascribed type of export is not compatible")


;; --- Real components, whose exports refer to what they define or import.

(component (component
  (component $c (type $r (resource (rep i32))) (export "r" (type $r)))
  (export "out" (component $c) (component (export "r" (type (sub resource)))))
))

(component (component
  (component $c (import "r" (type $r (sub resource))) (export "e" (type $r)))
  (export "out" (component $c) (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
))

(component (component
  (component $c (import "r" (type $r (sub resource))) (export "e" (type $r)))
  (export "out" (component $c) (component (import "r" (type (sub resource))) (export "e" (type (sub resource)))))
))

(assert_invalid
  (component (component
  (component $c (import "r" (type (sub resource))) (type $d (resource (rep i32))) (export "e" (type $d)))
  (export "out" (component $c) (component (import "r" (type $r (sub resource))) (export "e" (type (eq $r)))))
))
  "ascribed type of export is not compatible")

(assert_invalid
  (component (component
  (component $c (type $r (resource (rep i32))) (type $s (resource (rep i32))) (export "r" (type $r)) (export "s" (type $s)))
  (export "out" (component $c) (component (export "r" (type $e (sub resource))) (export "s" (type (eq $e)))))
))
  "ascribed type of export is not compatible")
