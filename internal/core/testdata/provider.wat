(component
  (core module $m
    (func (export "double") (param i32) (result i32)
      local.get 0
      i32.const 2
      i32.mul
    )
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 0)
  )
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $realloc))
  (alias core export $i "double" (core func $double))
  (type $ft (func (param "x" s32) (result s32)))
  (func $f (type $ft) (canon lift (core func $double) (memory $mem) (realloc $realloc)))
  (export "double" (func $f))
)
