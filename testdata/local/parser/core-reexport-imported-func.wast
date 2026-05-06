;; RUN: wast --assert default --snapshot tests/snapshots %

;; Regression test: a core module that re-exports an imported function must
;; carry that import's type through to downstream consumers. Earlier the
;; validator stored a placeholder zero-valued CoreFuncTypeID for re-exports of
;; imports, which silently aliased whatever happened to be at arena slot 0.
;;
;; The "$bias" module below is parsed first so its defined func pushes a
;; non-`(func)` signature into arena slot 0. If the placeholder bug ever
;; returns, the final instantiation will mismatch against that slot.

(component
  ;; Push a 4-param/1-result core func type into the arena before anything else.
  (core module $bias
    (func (export "x") (param i32 i32 i32 i32) (result i32)
      i32.const 0
    )
  )

  ;; Provider: a core module that defines and exports `f` with type (func).
  (core module $provider
    (func (export "f"))
  )
  (core instance $p (instantiate $provider))

  ;; Shim: imports `f` of type (func) and re-exports it under the same name.
  (core module $shim
    (type (func))
    (import "" "f" (func (type 0)))
    (export "f" (func 0))
  )
  (core instance $s (instantiate $shim
    (with "" (instance $p))
  ))

  ;; Repackage $s's exports into a fresh core instance via inline exports;
  ;; this exercises coreInstantiateFromExports' propagation of the func type.
  (core instance $repack
    (export "f" (func $s "f"))
  )

  ;; Importer: requires `f` of type (func). Validation here checks the type
  ;; that flowed through both the alias and the inline-exports instance.
  (core module $importer
    (type (func))
    (import "" "f" (func (type 0)))
  )
  (core instance (instantiate $importer
    (with "" (instance $repack))
  ))
)
