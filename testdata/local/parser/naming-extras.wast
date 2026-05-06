;; Local-only extras for naming.wast (not in upstream WebAssembly/component-model
;; test/wasm-tools/naming.wast). Run alongside the spec mirror by the
;; wasmparser test runner.
;;
;; Verifies that flag names cannot start with a digit (stricter kebab-case
;; check than the upstream test covers).

(assert_invalid
  (component
    (type (flags "0-a-1-c"))
  )
  "flag name `0-a-1-c` is not in kebab case"
)

(component
  (type (flags "a-1-c"))
)
