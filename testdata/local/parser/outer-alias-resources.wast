;; Local-only tests: an outer alias crossing a component boundary can't carry any
;; free resource, wherever the resource came from. Resources a type defines itself
;; aren't free. Each valid case is nested so that its imports don't need a provider.
;; Expectations were checked against wasm-tools 1.254.0.

;; --- Resources from outside the component are rejected, whatever their origin.

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (component (alias outer 1 $r (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "r" (type $r (sub resource)))
    (component (alias outer 1 $r (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "r" (type $r (sub resource)))
    (export $e "e" (type $r))
    (component (alias outer 1 $e (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (component $s
      (type $t (resource (rep i32)))
      (export "r" (type $t))
    )
    (instance $i (instantiate $s))
    (alias export $i "r" (type $r))
    (component (alias outer 1 $r (type $x)))
  )
  "refers to resources not defined in the current component")

;; --- So are types mentioning one.

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $o (own $r))
    (type $l (list $o))
    (component (alias outer 1 $l (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $o (own $r))
    (type $f (func (param "h" $o)))
    (component (alias outer 1 $f (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $i (instance
      (alias outer 1 $r (type $r2))
      (export "r" (type (eq $r2)))
    ))
    (component (alias outer 1 $i (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $i (instance
      (export "n" (instance
        (alias outer 2 $r (type $r2))
        (export "r" (type (eq $r2)))
      ))
    ))
    (component (alias outer 1 $i (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $c (component
      (alias outer 1 $r (type $r2))
      (import "r" (type (eq $r2)))
    ))
    (component (alias outer 1 $c (type $x)))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $c (component
      (alias outer 1 $r (type $r2))
      (export "r" (type (eq $r2)))
    ))
    (component (alias outer 1 $c (type $x)))
  )
  "refers to resources not defined in the current component")

;; A type defining its own resource still can't hide an outside one.
(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (type $i (instance
      (export "s" (type (sub resource)))
      (alias outer 1 $r (type $r2))
      (export "r" (type (eq $r2)))
    ))
    (component (alias outer 1 $i (type $x)))
  )
  "refers to resources not defined in the current component")

;; --- Every boundary on the way out counts.

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (component (component (alias outer 2 $r (type $x))))
  )
  "refers to resources not defined in the current component")

(assert_invalid
  (component
    (import "a" (instance $a (export "r" (type (sub resource)))))
    (alias export $a "r" (type $r))
    (component (type (instance (alias outer 2 $r (type $x)))))
  )
  "refers to resources not defined in the current component")

;; --- Types with no free resources may cross.

(component (type $t u32) (component (alias outer 1 $t (type $x))))

(component
  (type $t (record (field "x" u32)))
  (component (alias outer 1 $t (type $x)))
)

(component
  (type $i (instance (export "r" (type (sub resource)))))
  (component (alias outer 1 $i (type $x)))
)

(component
  (type $i (instance
    (export "r" (type $r (sub resource)))
    (export "s" (type (eq $r)))
    (type $o (own $r))
    (type $l (list $o))
    (export "l" (type (eq $l)))
  ))
  (component (alias outer 1 $i (type $x)))
)

(component
  (type $i (instance (export "n" (instance (export "r" (type (sub resource)))))))
  (component (alias outer 1 $i (type $x)))
)

(component
  (type $i (instance
    (export "r" (type $r (sub resource)))
    (export "n" (instance
      (alias outer 1 $r (type $r2))
      (export "x" (type (eq $r2)))
    ))
  ))
  (component (alias outer 1 $i (type $x)))
)

(component
  (type $i (instance
    (export "n" (instance $n (export "r" (type (sub resource)))))
    (alias export $n "r" (type $r))
    (export "e" (type (eq $r)))
  ))
  (component (alias outer 1 $i (type $x)))
)

(component
  (type $c (component
    (import "r" (type $r (sub resource)))
    (type $o (own $r))
    (export "e" (type (eq $o)))
  ))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component (export "r" (type (sub resource)))))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component
    (import "r" (type $r (sub resource)))
    (export "e" (type $e (eq $r)))
    (export "f" (type (eq $e)))
  ))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component
    (import "i" (instance $i (export "r" (type (sub resource)))))
    (alias export $i "r" (type $r))
    (export "e" (type (eq $r)))
  ))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component
    (import "i" (instance $i (export "n" (instance (export "r" (type (sub resource)))))))
    (alias export $i "n" (instance $n))
    (alias export $n "r" (type $r))
    (export "e" (type (eq $r)))
  ))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component
    (export "i" (instance $i (export "r" (type (sub resource)))))
    (alias export $i "r" (type $r))
    (export "e" (type (eq $r)))
  ))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component (export "c" (component (import "r" (type (sub resource)))))))
  (component (alias outer 1 $c (type $x)))
)

;; An eq-bound import gets its own ID, which is still the same resource.
(component
  (type $c (component
    (import "r" (type $r (sub resource)))
    (import "s" (type $s (eq $r)))
    (export "e" (type (eq $s)))
  ))
  (component (alias outer 1 $c (type $x)))
)

(component
  (type $c (component
    (import "r" (type $r (sub resource)))
    (import "s" (type $s (eq $r)))
    (type $o (own $s))
    (type $b (borrow $s))
    (export "o" (type (eq $o)))
    (export "b" (type (eq $b)))
    (export "l" (func (param "x" $o)))
  ))
  (component (alias outer 1 $c (type $x)))
)

;; A component type copied by importing the instance exporting it keeps what it
;; defines.
(component (component
  (import "i" (instance $i
    (type $c (component (export "r" (type (sub resource)))))
    (export "c" (type (eq $c)))
    (export "s" (type (sub resource)))
  ))
  (alias export $i "c" (type $c))
  (component (alias outer 1 $c (type $x)))
))

(component (component
  (import "i" (instance $i
    (type $c (component (import "r" (type (sub resource)))))
    (export "c" (type (eq $c)))
    (export "s" (type (sub resource)))
  ))
  (alias export $i "c" (type $c))
  (component (alias outer 1 $c (type $x)))
))

;; --- Entering a type declaration crosses no component boundary.

(component (component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type (instance
    (alias outer 1 $r (type $x))
    (export "r" (type (eq $x)))
  ))
))

(component (component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type (component (type (instance (alias outer 2 $r (type $x))))))
))

(component
  (component
    (type $t (resource (rep i32)))
    (type (instance (alias outer 1 $t (type $x))))
  )
)
