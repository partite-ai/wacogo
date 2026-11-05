;; An outer alias can only refer to a preceding definition. A component being
;; defined isn't one, as it enters its parent's index space where it ends.

;; itself
(assert_invalid
  (component
    (component $u
      (alias outer 1 0 (component $c))))
  "unknown component 0: component index out of bounds")

;; itself, instantiated
(assert_invalid
  (component
    (component $u
      (alias outer 1 0 (component $c))
      (instance (instantiate $c))))
  "unknown component 0: component index out of bounds")

;; a component defined after it
(assert_invalid
  (component
    (component $u
      (alias outer 1 1 (component $c)))
    (component))
  "unknown component 1: component index out of bounds")

;; an enclosing component, from further down
(assert_invalid
  (component
    (component $u
      (component
        (alias outer 2 0 (component $c)))))
  "unknown component 0: component index out of bounds")

;; its parent, which is being defined in turn
(assert_invalid
  (component
    (component $a)
    (component $u
      (component
        (alias outer 2 1 (component $c)))))
  "unknown component 1: component index out of bounds")

;; a preceding sibling
(component
  (component $a)
  (component $u
    (alias outer 1 $a (component $c))
    (instance (instantiate $c)))
  (instance (instantiate $u)))

;; a preceding definition from further down
(component
  (component $a)
  (component $u
    (component $v
      (alias outer 2 $a (component $c))
      (instance (instantiate $c)))
    (instance (instantiate $v)))
  (instance (instantiate $u)))

;; a sibling, once it has ended
(component
  (component $a
    (component $inner))
  (component $b
    (alias outer 1 $a (component $c)))
  (alias outer 0 $b (component $again))
  (instance (instantiate $again)))
