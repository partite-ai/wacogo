;; Local-only tests: a component using a feature the engine doesn't enable is
;; rejected when it's validated. Most other features fail earlier, in the parser,
;; which doesn't know their syntax; these two get past it.
;; Each expectation was checked against wasm-tools 1.254.0 and wasmtime 50.0.1.

;; --- error-context, wherever a type can mention it.

(assert_invalid
  (component (type error-context))
  "`error-context` requires the component model error-context feature")

(assert_invalid
  (component (type (list error-context)))
  "`error-context` requires the component model error-context feature")

(assert_invalid
  (component (type (record (field "x" error-context))))
  "`error-context` requires the component model error-context feature")

(assert_invalid
  (component (type (func (param "x" error-context))))
  "`error-context` requires the component model error-context feature")

(assert_invalid
  (component (import "f" (func (param "x" error-context))))
  "`error-context` requires the component model error-context feature")

(assert_invalid
  (component (import "f" (func (result error-context))))
  "`error-context` requires the component model error-context feature")

;; in a declared instance type, aliased out, where an instance may omit the type
(assert_invalid
  (component
    (import "i" (instance $i
      (type $l (list error-context))
      (export "t" (type (eq $l)))))
    (alias export $i "t" (type $t)))
  "`error-context` requires the component model error-context feature")

;; on a lifted func
(assert_invalid
  (component
    (core module $m (func (export "f") (param i32)))
    (core instance $i (instantiate $m))
    (func (export "f") (param "x" error-context) (canon lift (core func $i "f"))))
  "`error-context` requires the component model error-context feature")

;; --- Values.

;; an import, used
(assert_invalid
  (component
    (import "v" (value $v string))
    (export "e" (value $v)))
  "support for component model `value`s is not enabled")

;; an import, unused
(assert_invalid
  (component (import "v" (value string)))
  "support for component model `value`s is not enabled")

;; in a declared instance type
(assert_invalid
  (component (type (instance (export "v" (value string)))))
  "support for component model `value`s is not enabled")

;; in a declared component type
(assert_invalid
  (component (type (component (import "v" (value string)))))
  "support for component model `value`s is not enabled")

;; an export, with no way for a value to exist
(assert_invalid
  (component (export "e" (value 0)))
  "value index out of bounds")

;; a start func, which is how values get consumed and produced
(assert_invalid
  (component
    (core module $m (func (export "g")))
    (core instance $i (instantiate $m))
    (func $s (canon lift (core func $i "g")))
    (start $s))
  "support for component model `value`s is not enabled")
