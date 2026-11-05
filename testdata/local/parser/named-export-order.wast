;; Local-only tests: an import or export can only use names established by what's
;; declared before it. Each export of an instance names its types once it has been
;; validated itself, in declaration order and at any depth.
;; Each expectation was checked against wasm-tools 1.254.0 and wasmtime 50.0.1.

;; a type naming a resource from an earlier nested instance
(component
  (import "i" (instance $i
    (export "n" (instance $n (export "r" (type (sub resource)))))
    (alias export $n "r" (type $r))
    (type $o (own $r))
    (export "o" (type (eq $o))))))

;; through two levels, with names that sort the other way
(component
  (import "i" (instance $i
    (export "z" (instance $m (export "y" (instance (export "r" (type (sub resource)))))))
    (alias export $m "y" (instance $n))
    (alias export $n "r" (type $r))
    (type $o (own $r))
    (export "a" (type (eq $o))))))

;; a list of a record an earlier nested instance named, through that name
(component
  (type $rec (record (field "x" u32)))
  (import "i" (instance $i
    (alias outer 1 $rec (type $x))
    (export "n" (instance $n (alias outer 1 $x (type $y)) (export "x" (type (eq $y)))))
    (alias export $n "x" (type $z))
    (type $l (list $z))
    (export "a" (type (eq $l))))))

;; a func after what names its type, through that name
(component
  (type $rec (record (field "x" u32)))
  (import "i" (instance $i
    (alias outer 1 $rec (type $x))
    (export "z" (type $z (eq $x)))
    (type $f (func (param "x" $z)))
    (export "a" (func (type $f))))))

;; --- Named too late.

;; by a later nested instance
(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (type $l (list $x))
      (export "a" (type (eq $l)))
      (export "n" (instance (alias outer 1 $x (type $y)) (export "x" (type (eq $y))))))))
  "instance not valid to be used as import")

;; by a later export of the same instance
(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (type $l (list $x))
      (export "a" (type (eq $l)))
      (export "x" (type (eq $x))))))
  "instance not valid to be used as import")

;; a func before the export naming its type
(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (type $f (func (param "x" $x)))
      (export "a" (func (type $f)))
      (export "z" (type (eq $x))))))
  "instance not valid to be used as import")

;; never named
(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (type $l (list $x))
      (export "a" (type (eq $l))))))
  "instance not valid to be used as import")

;; --- The same for an instance exported, whichever way its type was built.

;; an inline instance, in order
(component
  (component $k
    (type $r (resource (rep i32)))
    (export $e "r" (type $r))
    (type $o (own $e))
    (instance $n (export "z" (type $e)) (export "a" (type $o)))
    (export "n" (instance $n))))

;; an inline instance, too late
(assert_invalid
  (component
    (component $k
      (type $r (resource (rep i32)))
      (type $o (own $r))
      (instance $n (export "a" (type $o)) (export "z" (type $r)))
      (export "n" (instance $n))))
  "instance not valid to be used as export")

;; a sub-instance keeps its component's export order
(component
  (component $k
    (component $s
      (type $r (resource (rep i32)))
      (export $e "z" (type $r))
      (type $o (own $e))
      (export "a" (type $o)))
    (instance $n (instantiate $s))
    (export "n" (instance $n))))

;; a remapped instance keeps its order
(component
  (component $k
    (type $it (instance
      (export "z" (type $r (sub resource)))
      (type $o (own $r))
      (export "a" (type (eq $o)))))
    (import "x" (instance (type $it)))
    (import "y" (instance (type $it)))))
;; --- A name belongs to the reference an (eq) export creates, not to the type it
;; refers to, which stays unnamed through its own index.

(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (export "z" (type (eq $x)))
      (type $f (func (param "x" $x)))
      (export "a" (func (type $f))))))
  "instance not valid to be used as import")

(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (export "z" (type (eq $x)))
      (type $l (list $x))
      (export "a" (type (eq $l))))))
  "instance not valid to be used as import")

(assert_invalid
  (component
    (type $rec (record (field "x" u32)))
    (import "i" (instance $i
      (alias outer 1 $rec (type $x))
      (export "n" (instance (alias outer 1 $x (type $y)) (export "x" (type (eq $y)))))
      (type $l (list $x))
      (export "a" (type (eq $l))))))
  "instance not valid to be used as import")

;; the same for a resource: its own export names it
(component
  (import "i" (instance $i
    (export "r" (type $r (sub resource)))
    (type $o (own $r))
    (export "o" (type (eq $o))))))

;; a resource its own import already named, so $x is named however it is referred to
(component
  (import "s" (type $s (sub resource)))
  (import "i" (instance $i
    (alias outer 1 $s (type $x))
    (export "z" (type (eq $x)))
    (type $o (own $x))
    (export "o" (type (eq $o))))))

;; --- Comparing looks through references: they're still the same type.

;; two imports linked by an (eq) export of a shared type
(component
  (component $wrap
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (import "b" (instance $b (export "x" (type (eq $r)))))
    (import "c" (instance $c (export "y" (type (eq $r)))))
    (alias export $b "x" (type $bx))
    (alias export $c "y" (type $cy))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $bx)) (with "q" (type $cy))))))

;; in a declared instance, an (eq) of an earlier export is that resource
(component
  (component $wrap
    (import "i" (instance $i
      (export "r" (type $r (sub resource)))
      (export "e" (type (eq $r)))))
    (alias export $i "r" (type $ir))
    (alias export $i "e" (type $ie))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $ir)) (with "q" (type $ie))))))

;; a record through two references is one type
(component
  (component $wrap
    (type $rec (record (field "x" u32)))
    (export $e "rec" (type $rec))
    (import "c" (component $c
      (alias outer 1 $e (type $r2))
      (import "i" (instance (export "t" (type (eq $r2)))))))
    (type $other (record (field "x" u32)))
    (export $o "other" (type $other))
    (instance $s (export "t" (type $o)))
    (instance (instantiate $c (with "i" (instance $s))))))

;; a declared export's reference survives instantiation
(component
  (component $wrap
    (import "s" (type $s (sub resource)))
    (import "c" (component $c
      (alias outer 1 $s (type $s2))
      (export "x" (type (eq $s2)))
      (export "y" (type (eq $s2)))))
    (instance $i (instantiate $c))
    (alias export $i "x" (type $ix))
    (alias export $i "y" (type $iy))
    (component $need
      (import "p" (type $p (sub resource)))
      (import "q" (type (eq $p)))
      (import "w" (type (eq $p))))
    (instance (instantiate $need (with "p" (type $s)) (with "q" (type $ix)) (with "w" (type $iy))))))
