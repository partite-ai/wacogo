;; Local-only tests: an export's ascribed type must name the exported item's
;; resources by identity. Unlike instantiation, an ascription binds nothing, so
;; it can't name another resource or introduce a fresh one, even inside an
;; instance type. Each case is nested so that its imports don't need a provider.
;; Expectations were checked against wasm-tools 1.254.0.

;; --- Naming the same resource is fine.

(component (component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $ra))
  (export "x" (type $ra) (type (eq $ra)))
))

(component (component
  (type $r (resource (rep i32)))
  (export "x" (type $r) (type (eq $r)))
))

(component (component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $ra))
  (export $e "e" (type $ra))
  (type $o (own $ra))
  (type $l (list $o))
  (type $o2 (own $e))
  (type $l2 (list $o2))
  (export "x" (type $l) (type (eq $l2)))
))

(component (component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $ra))
  (core module $m (func (export "f") (param i32)))
  (core instance $mi (instantiate $m))
  (type $o (own $ra))
  (func $f (param "h" $o) (canon lift (core func $mi "f")))
  (export "f" (func $f) (func (param "h" $o)))
))

(component (component
  (import "i" (instance $i (export "r" (type (sub resource)))))
  (alias export $i "r" (type $r))
  (export "x" (instance $i) (instance
    (alias outer 1 $r (type $r2))
    (export "r" (type (eq $r2)))
  ))
))

;; --- Naming a different resource is not.

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (import "b" (instance $b (export "r" (type (sub resource)))))
    (alias export $a "r" (type $ra))
    (alias export $b "r" (type $rb))
    (export "x" (type $ra) (type (eq $rb)))
  )
  "resource types are not the same")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (import "b" (instance $b (export "r" (type (sub resource)))))
    (alias export $a "r" (type $ra))
    (alias export $b "r" (type $rb))
    (export $e "e" (type $rb))
    (type $o (own $ra))
    (type $l (list $o))
    (type $o2 (own $e))
    (type $l2 (list $o2))
    (export "x" (type $l) (type (eq $l2)))
  )
  "resource types are not the same")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (import "b" (instance $b (export "r" (type (sub resource)))))
    (alias export $a "r" (type $ra))
    (alias export $b "r" (type $rb))
    (core module $m (func (export "f") (param i32)))
    (core instance $mi (instantiate $m))
    (type $o (own $ra))
    (type $p (own $rb))
    (func $f (param "h" $o) (canon lift (core func $mi "f")))
    (export "f" (func $f) (func (param "h" $p)))
  )
  "ascribed type of export is not compatible")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (import "b" (instance $b (export "r" (type (sub resource)))))
    (alias export $b "r" (type $rb))
    (export "x" (instance $a) (instance
      (alias outer 1 $rb (type $r2))
      (export "r" (type (eq $r2)))
    ))
  )
  "ascribed type of export is not compatible")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (import "b" (instance $b (export "r" (type (sub resource)))))
    (alias export $a "r" (type $ra))
    (alias export $b "r" (type $rb))
    (import "i" (instance $i (export "n" (instance
      (alias outer 2 $ra (type $r2))
      (export "r" (type (eq $r2)))
    ))))
    (export "x" (instance $i) (instance (export "n" (instance
      (alias outer 2 $rb (type $r3))
      (export "r" (type (eq $r3)))
    ))))
  )
  "ascribed type of export is not compatible")

;; Each instantiation of a subcomponent defines its own resource.
(assert_invalid
  (component
    (component $s
      (type $r (resource (rep i32)))
      (export "r" (type $r))
    )
    (instance $x1 (instantiate $s))
    (instance $x2 (instantiate $s))
    (alias export $x1 "r" (type $r1))
    (alias export $x2 "r" (type $r2))
    (export "x" (type $r1) (type (eq $r2)))
  )
  "resource types are not the same")

(assert_invalid
  (component
    (component $sub
      (type $r (resource (rep i32)))
      (export $e "r" (type $r))
      (type $o (own $e))
      (type $l (list $o))
      (export "l" (type $l))
    )
    (instance $x1 (instantiate $sub))
    (instance $x2 (instantiate $sub))
    (alias export $x2 "r" (type $r2))
    (alias export $x1 "l" (type $l1))
    (export $e2 "r2" (type $r2))
    (type $o2 (own $e2))
    (type $x2l (list $o2))
    (export "l1" (type $l1) (type (eq $x2l)))
  )
  "resource types are not the same")

;; --- An ascription can't introduce a fresh resource.

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $ra))
    (export "x" (type $ra) (type (sub resource)))
  )
  "resource types are not the same")

(assert_invalid
  (component
    (type $r (resource (rep i32)))
    (export "x" (type $r) (type (sub resource)))
  )
  "resource types are not the same")

(assert_invalid
  (component
    (import "i" (instance $i (export "r" (type (sub resource)))))
    (export "x" (instance $i) (instance (export "r" (type (sub resource)))))
  )
  "ascribed type of export is not compatible")

(assert_invalid
  (component
    (import "i" (instance $i (export "n" (instance (export "r" (type (sub resource)))))))
    (export "x" (instance $i) (instance (export "n" (instance (export "r" (type (sub resource)))))))
  )
  "ascribed type of export is not compatible")
