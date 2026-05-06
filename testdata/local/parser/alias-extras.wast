;; Local-only extras for alias.wast (not in upstream WebAssembly/component-model
;; test/wasm-tools/alias.wast). Run alongside the spec mirror by the
;; wasmparser test runner.

(assert_invalid
  (component
    (type (component
      (import "x" (instance $I (export "f" (func))))
      (alias export $I "f" (func))
    ))
  )
  "aliases in a component or instance type may only refer to types or instances")
