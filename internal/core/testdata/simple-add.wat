(component
  (core module $m
    (func (export "add") (param i32 i32) (result i32)
      local.get 0
      local.get 1
      i32.add
    )
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      i32.const 0
    )
  )
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $realloc))
  (alias core export $i "add" (core func $add))
  (type $ft (func (param "a" s32) (param "b" s32) (result s32)))
  (func $lifted (type $ft) (canon lift (core func $add) (memory $mem) (realloc $realloc)))
  (export "add" (func $lifted))
)
