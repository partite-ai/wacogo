;; Local-only tests: instantiating a component or importing an instance replaces
;; only the resources it creates or its imports bind. Everything else keeps its
;; identity, however deep in a type it is referred to.
;; Each expectation was checked against wasm-tools 1.254.0. Where wasmtime 50.0.1
;; differs, a comment says so: it refuses to compile any type export that isn't a
;; resource or value type ("unsupported type export"), whatever its contents.

;; --- An import keeps resources from outside it. Instantiating $need proves it:
;; it requires the func's parameter to be the resource passed alongside it.

;; reached only through a func type (wasmtime: unsupported type export)
(component
  (import "i" (instance $i
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $f (func (param "x" $o)))
    (export "ft" (type (eq $f)))))
  (alias export $i "r" (type $r))
  (alias export $i "ft" (type $ft))
  (import "j" (instance $j
    (alias outer 1 $ft (type $g))
    (export "f" (func (type $g)))))
  (alias export $j "f" (func $f))
  (component $need
    (import "a" (type $a (sub resource)))
    (type $oa (own $a))
    (import "f" (func (param "x" $oa))))
  (instance (instantiate $need (with "a" (type $r)) (with "f" (func $f)))))

;; the same with a resource the import defines alongside it
;; (wasmtime: unsupported type export)
(component
  (import "i" (instance $i
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $f (func (param "x" $o)))
    (export "ft" (type (eq $f)))))
  (alias export $i "r" (type $r))
  (alias export $i "ft" (type $ft))
  (import "j" (instance $j
    (alias outer 1 $ft (type $g))
    (export "f" (func (type $g)))
    (export "s" (type (sub resource)))))
  (alias export $j "f" (func $f))
  (component $need
    (import "a" (type $a (sub resource)))
    (type $oa (own $a))
    (import "f" (func (param "x" $oa))))
  (instance (instantiate $need (with "a" (type $r)) (with "f" (func $f)))))

;; reached only through an instance type (wasmtime: unsupported type export)
(component
  (import "i" (instance $i
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $it (instance (export "o" (type (eq $o)))))
    (export "it" (type (eq $it)))))
  (alias export $i "it" (type $it))
  (import "j" (instance (alias outer 1 $it (type $it2)) (export "n" (instance (type $it2))))))

;; a record stays the same type, so it stays named
(component
  (import "i" (instance $i
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $rec (record (field "x" $o)))
    (export "t" (type $t (eq $rec)))))
  (alias export $i "t" (type $t))
  (import "j" (instance
    (alias outer 1 $t (type $t2))
    (export "t" (type $t3 (eq $t2)))
    (export "s" (type $s (sub resource)))
    (type $os (own $s))
    (type $f (func (param "x" $t3) (param "y" $os)))
    (export "f" (func (type $f))))))

;; an own type stays the same type alongside a resource the import defines
(component
  (import "i" (instance $i (export "r" (type $r (sub resource)))))
  (alias export $i "r" (type $r))
  (type $o (own $r))
  (import "j" (instance
    (alias outer 1 $o (type $o2))
    (export "o" (type $o3 (eq $o2)))
    (export "s" (type $s (sub resource)))
    (type $os (own $s))
    (type $f (func (param "x" $o3) (param "y" $os)))
    (export "f" (func (type $f))))))

;; --- What an instance type defines is new with each import, at any depth.

;; in the instance itself
(assert_invalid
  (component
    (type $it (instance (export "r" (type (sub resource)))))
    (import "a" (instance $a (type $it)))
    (import "b" (instance $b (type $it)))
    (alias export $a "r" (type $ar))
    (alias export $b "r" (type $br))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $ar)) (with "q" (type $br)))))
  "resource types are not the same")

;; in a nested instance
(assert_invalid
  (component
    (type $it (instance (export "n" (instance (export "r" (type (sub resource)))))))
    (import "a" (instance $a (type $it)))
    (import "b" (instance $b (type $it)))
    (alias export $a "n" (instance $an))
    (alias export $b "n" (instance $bn))
    (alias export $an "r" (type $ar))
    (alias export $bn "r" (type $br))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $ar)) (with "q" (type $br)))))
  "resource types are not the same")

;; a nested instance whose only tie to the resource is a func type must follow it
;; to its new identity
(assert_invalid
  (component
    (type $it (instance
      (export "r" (type $r (sub resource)))
      (type $o (own $r))
      (type $f (func (param "x" $o)))
      (export "n" (instance (export "ft" (type (eq $f)))))))
    (import "a" (instance $a (type $it)))
    (import "b" (instance $b (type $it)))
    (alias export $a "n" (instance $an))
    (alias export $b "r" (type $br))
    (alias export $an "ft" (type $ft))
    (component $need
      (import "p" (type $p (sub resource)))
      (type $op (own $p))
      (type $want (func (param "x" $op)))
      (import "q" (type (eq $want))))
    (instance (instantiate $need (with "p" (type $br)) (with "q" (type $ft)))))
  "")

;; and that same func type is the one its own instance's resource names
;; (wasmtime: unsupported type export)
(component
  (type $it (instance
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $f (func (param "x" $o)))
    (export "n" (instance (export "ft" (type (eq $f)))))))
  (import "a" (instance $a (type $it)))
  (alias export $a "n" (instance $an))
  (alias export $a "r" (type $ar))
  (alias export $an "ft" (type $ft))
  (component $need
    (import "p" (type $p (sub resource)))
    (type $op (own $p))
    (type $want (func (param "x" $op)))
    (import "q" (type (eq $want))))
  (instance (instantiate $need (with "p" (type $ar)) (with "q" (type $ft)))))

;; --- Instantiating a declared component keeps a resource from outside its type.

;; its export is the outer resource
(component
  (component $wrap
    (import "r" (type $r (sub resource)))
    (import "c" (component $c
      (alias outer 1 $r (type $r2))
      (export "x" (type (eq $r2)))))
    (instance $i (instantiate $c))
    (alias export $i "x" (type $x))
    (component $need
      (import "a" (type $a (sub resource)))
      (import "b" (type (eq $a))))
    (instance (instantiate $need (with "a" (type $r)) (with "b" (type $x))))))

;; and not some other one
(assert_invalid
  (component
    (component $wrap
      (import "r" (type $r (sub resource)))
      (import "s" (type $s (sub resource)))
      (import "c" (component $c
        (alias outer 1 $r (type $r2))
        (export "x" (type (eq $r2)))))
      (instance $i (instantiate $c))
      (alias export $i "x" (type $x))
      (component $need
        (import "a" (type $a (sub resource)))
        (import "b" (type (eq $a))))
      (instance (instantiate $need (with "a" (type $s)) (with "b" (type $x))))))
  "resource types are not the same")

;; --- An exported component type follows what its parent's imports were given.

(component
  (component $wrap
    (import "r" (type $r (sub resource)))
    (component $k
      (import "s" (type $s (sub resource)))
      (import "c" (component $c
        (alias outer 1 $s (type $s2))
        (export "x" (type (eq $s2)))))
      (export "c" (component $c)))
    (import "c" (component $c
      (alias outer 1 $r (type $r2))
      (export "x" (type (eq $r2)))))
    (instance $i (instantiate $k (with "s" (type $r)) (with "c" (component $c))))
    (alias export $i "c" (component $d))
    (instance $j (instantiate $d))
    (alias export $j "x" (type $x))
    (component $need
      (import "a" (type $a (sub resource)))
      (import "b" (type (eq $a))))
    (instance (instantiate $need (with "a" (type $r)) (with "b" (type $x))))))

;; --- What a component creates is new with each instantiation.

(assert_invalid
  (component
    (component $k (type $r (resource (rep i32))) (export "r" (type $r)))
    (instance $a (instantiate $k))
    (instance $b (instantiate $k))
    (alias export $a "r" (type $ar))
    (alias export $b "r" (type $br))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $ar)) (with "q" (type $br)))))
  "resource types are not the same")

;; including when a sub-instance creates it, and it arrives as an alias
(assert_invalid
  (component
    (component $k (type $r (resource (rep i32))) (export "r" (type $r)))
    (component $m
      (import "k" (component $kk (export "r" (type (sub resource)))))
      (instance $i (instantiate $kk))
      (alias export $i "r" (type $x))
      (export "x" (type $x)))
    (instance $a (instantiate $m (with "k" (component $k))))
    (instance $b (instantiate $m (with "k" (component $k))))
    (alias export $a "x" (type $ax))
    (alias export $b "x" (type $bx))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $ax)) (with "q" (type $bx)))))
  "resource types are not the same")

;; and one instantiation's two names for it are still one resource
(component
  (component $k
    (type $r (resource (rep i32)))
    (export $x "x" (type $r))
    (export "y" (type $x)))
  (instance $a (instantiate $k))
  (alias export $a "x" (type $ax))
  (alias export $a "y" (type $ay))
  (component $need
    (import "p" (type $p (sub resource)))
    (import "q" (type (eq $p))))
  (instance (instantiate $need (with "p" (type $ax)) (with "q" (type $ay)))))

;; --- A type nothing was replaced in isn't copied, even when a func holding it is.
;; A copy of this record would be a new type with no name, which an import's func
;; can't refer to.

(component
  (import "i" (instance $i
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $rec (record (field "x" $o)))
    (export "t" (type $t (eq $rec)))))
  (alias export $i "t" (type $t))
  (import "j" (instance
    (alias outer 1 $t (type $t2))
    (export "s" (type $s (sub resource)))
    (type $os (own $s))
    (type $f (func (param "x" $t2) (param "y" $os)))
    (export "f" (func (type $f))))))

;; --- A nested instance whose only tie to the resource is an instance type follows
;; it to its new identity. (wasmtime: unsupported type export)

(component
  (type $it (instance
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (type $t (instance (export "o" (type (eq $o)))))
    (export "n" (instance (export "t" (type (eq $t)))))))
  (import "a" (instance $a (type $it)))
  (alias export $a "n" (instance $an))
  (alias export $a "r" (type $ar))
  (alias export $an "t" (type $t))
  (component $need
    (import "p" (type $p (sub resource)))
    (type $op (own $p))
    (type $want (instance (export "o" (type (eq $op)))))
    (import "q" (type (eq $want))))
  (instance (instantiate $need (with "p" (type $ar)) (with "q" (type $t)))))

;; --- A nested instance whose only tie to the resource is a component follows it.

(component
  (component $wrap
    (import "r" (type $r (sub resource)))
    (component $k
      (import "s" (type $s (sub resource)))
      (import "c" (component $c
        (alias outer 1 $s (type $s2))
        (export "x" (type (eq $s2)))))
      (instance $h (export "c" (component $c)))
      (export "h" (instance $h)))
    (import "c" (component $c
      (alias outer 1 $r (type $r2))
      (export "x" (type (eq $r2)))))
    (instance $i (instantiate $k (with "s" (type $r)) (with "c" (component $c))))
    (alias export $i "h" (instance $ih))
    (alias export $ih "c" (component $d))
    (instance $j (instantiate $d))
    (alias export $j "x" (type $x))
    (component $need
      (import "a" (type $a (sub resource)))
      (import "b" (type (eq $a))))
    (instance (instantiate $need (with "a" (type $r)) (with "b" (type $x))))))

;; --- A component type that was copied still creates its resources anew.

(assert_invalid
  (component
    (component $wrap
      (import "r" (type $r (sub resource)))
      (component $k
        (import "s" (type $s (sub resource)))
        (import "c" (component $c
          (alias outer 1 $s (type $s2))
          (export "x" (type (eq $s2)))
          (export "y" (type (sub resource)))))
        (export "c" (component $c)))
      (import "c" (component $c
        (alias outer 1 $r (type $r2))
        (export "x" (type (eq $r2)))
        (export "y" (type (sub resource)))))
      (instance $i (instantiate $k (with "s" (type $r)) (with "c" (component $c))))
      (alias export $i "c" (component $d))
      (instance $p (instantiate $d))
      (instance $q (instantiate $d))
      (alias export $p "y" (type $py))
      (alias export $q "y" (type $qy))
      (component $need
        (import "a" (type $a (sub resource)))
        (import "b" (type (eq $a))))
      (instance (instantiate $need (with "a" (type $py)) (with "b" (type $qy))))))
  "resource types are not the same")

;; --- An (eq) type import isn't left to the instantiator: it's the resource it names.

(assert_invalid
  (component
    (component $wrap
      (import "r" (type $r (sub resource)))
      (import "s" (type $s (sub resource)))
      (import "k" (component $k
        (alias outer 1 $s (type $s2))
        (import "c" (component
          (alias outer 1 $s2 (type $s3))
          (import "x" (type (eq $s3)))))))
      (import "c" (component $c
        (alias outer 1 $r (type $r2))
        (import "x" (type (eq $r2)))))
      (instance (instantiate $k (with "c" (component $c))))))
  "resource types are not the same")

(component
  (component $wrap
    (import "r" (type $r (sub resource)))
    (import "k" (component $k
      (alias outer 1 $r (type $s2))
      (import "c" (component
        (alias outer 1 $s2 (type $s3))
        (import "x" (type (eq $s3)))))))
    (import "c" (component $c
      (alias outer 1 $r (type $r2))
      (import "x" (type (eq $r2)))))
    (instance (instantiate $k (with "c" (component $c))))))
