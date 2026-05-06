(component
  (type $double-type (func (param "x" s32) (result s32)))
  (import "provider" (func $imported-double (type $double-type)))
  (core func $lowered-double (canon lower (func $imported-double)))
  (core module $m
    (import "" "double" (func $double (param i32) (result i32)))
    (func (export "quadruple") (param i32) (result i32)
      local.get 0
      call $double
      call $double
    )
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 0)
  )
  (core instance $bridge (export "double" (func $lowered-double)))
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $realloc))
  (alias core export $i "quadruple" (core func $quadruple))
  (type $ft (func (param "x" s32) (result s32)))
  (func $lifted (type $ft) (canon lift (core func $quadruple) (memory $mem) (realloc $realloc)))
  (export "quadruple" (func $lifted))
)
