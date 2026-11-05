;; Local-only tests: a component's imports can't name a resource the component
;; itself brings into existence, whether it declares it, a sub-instance gives it,
;; or a type declaration exports it. Nothing outside could supply such a resource.
;; Valid cases are nested so that their imports don't need a provider.
;; Expectations were checked against wasm-tools 1.254.0 and wasmtime.

;; --- Imports naming a resource defined here are rejected.

;; instance import equal to a local resource
(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (import "e" (instance (export "r" (type (eq $r))))))
  "local resource type found in imports")

;; type import equal to a local resource
(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (import "t" (type (eq $r))))
  "local resource type found in imports")

;; exported alias of a local resource
(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (export $x "r" (type $r))
    (import "e" (instance (export "r" (type (eq $x))))))
  "local resource type found in imports")

;; local resource through an inline instance
(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (instance $n (export "r" (type $r)))
    (alias export $n "r" (type $y))
    (import "t" (type (eq $y))))
  "local resource type found in imports")

;; resource created by a sub-instance
(assert_invalid
  (component
    (component $c
      (type $r (resource (rep i32)))
      (export "r" (type $r)))
    (instance $i (instantiate $c))
    (alias export $i "r" (type $r))
    (import "e" (instance (export "r" (type (eq $r))))))
  "local resource type found in imports")

;; resource created by a sub-instance's inline instance
(assert_invalid
  (component
    (component $c
      (type $r (resource (rep i32)))
      (instance $n (export "r" (type $r)))
      (export "n" (instance $n)))
    (instance $i (instantiate $c))
    (alias export $i "n" (instance $n))
    (alias export $n "r" (type $r))
    (import "t" (type (eq $r))))
  "local resource type found in imports")

;; resource created by an imported component
(assert_invalid
  (component
    (import "c" (component $c (export "r" (type (sub resource)))))
    (instance $i (instantiate $c))
    (alias export $i "r" (type $r))
    (import "e" (instance (export "r" (type (eq $r))))))
  "local resource type found in imports")

;; nested instance of an imported component
(assert_invalid
  (component
    (import "c" (component $c (export "n" (instance (export "r" (type (sub resource)))))))
    (instance $i (instantiate $c))
    (alias export $i "n" (instance $n))
    (alias export $n "r" (type $r))
    (import "t" (type (eq $r))))
  "local resource type found in imports")

;; in a nested component
(assert_invalid
  (component
    (component
      (type $r (resource (rep i32)))
      (import "e" (instance (export "r" (type (eq $r)))))))
  "local resource type found in imports")

;; inside an imported component's type
(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (import "c" (component (import "e" (instance (export "r" (type (eq $r))))))))
  "local resource type found in imports")

;; through an instance type declared outside the import
(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (type $i (instance
      (alias outer 1 $r (type $x))
      (export "r" (type (eq $x)))))
    (import "e" (instance (type $i))))
  "local resource type found in imports")

;; component type importing its exported resource
(assert_invalid
  (component
    (type (component
      (export "r" (type $r (sub resource)))
      (import "e" (instance (export "r" (type (eq $r))))))))
  "local resource type found in imports")

;; component type importing its exported instance's resource
(assert_invalid
  (component
    (type (component
      (export "i" (instance $i (export "r" (type (sub resource)))))
      (alias export $i "r" (type $r))
      (import "e" (instance (export "r" (type (eq $r))))))))
  "local resource type found in imports")

;; component type importing a nested exported instance's resource
(assert_invalid
  (component
    (type (component
      (export "i" (instance $i (export "n" (instance (export "r" (type (sub resource)))))))
      (alias export $i "n" (instance $n))
      (alias export $n "r" (type $r))
      (import "e" (instance (export "r" (type (eq $r))))))))
  "local resource type found in imports")

;; --- Imports naming only what is itself imported are fine.

;; imported resource named by another import
(component
  (component
    (import "a" (type $a (sub resource)))
    (import "t" (type (eq $a)))))

;; imported instance's resource named by another import
(component
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (import "b" (instance (export "r" (type (eq $r)))))))

;; re-exported import named by another import
(component
  (component
    (import "a" (type $a (sub resource)))
    (export $x "r" (type $a))
    (import "e" (instance (export "r" (type (eq $x)))))))

;; import forwarded by a sub-instance
(component
  (component
    (import "a" (type $a (sub resource)))
    (component $c
      (import "x" (type $x (sub resource)))
      (export "y" (type $x)))
    (instance $i (instantiate $c (with "x" (type $a))))
    (alias export $i "y" (type $y))
    (import "b" (instance (export "r" (type (eq $y)))))))

;; imported instance forwarded by a sub-instance
(component
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (component $c
      (import "x" (instance $x (export "r" (type (sub resource)))))
      (alias export $x "r" (type $r))
      (export "y" (type $r)))
    (instance $i (instantiate $c (with "x" (instance $a))))
    (alias export $i "y" (type $y))
    (import "b" (type (eq $y)))))

;; local resource no import names
(component
  (component
    (type $r (resource (rep i32)))
    (export "r" (type $r))
    (import "e" (instance (export "s" (type (sub resource)))))))

;; local resource only an unused type declaration names
(component
  (component
    (type $r (resource (rep i32)))
    (type (instance
      (alias outer 1 $r (type $x))
      (export "r" (type (eq $x)))))))

;; component type exporting what it imports
(component
  (component
    (type (component
      (import "r" (type $r (sub resource)))
      (export "e" (instance (export "r" (type (eq $r)))))))))

;; imported component with its own resources
(component
  (component
    (import "c" (component
      (export "r" (type $r (sub resource)))
      (export "e" (instance (export "r" (type (eq $r)))))))))
