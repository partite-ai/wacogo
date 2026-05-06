(component
  (core module $m
    (memory (export "memory") 1)
    (global $bump (mut i32) (i32.const 1024))
    (func (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      ;; align: ptr = (ptr + align - 1) & ~(align - 1)
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr)
    )
    ;; strlen: takes ptr and byte_len, returns byte_len
    (func (export "strlen") (param i32 i32) (result i32)
      local.get 1
    )
  )
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "cabi_realloc" (core func $realloc))
  (alias core export $i "strlen" (core func $strlen))
  (type $ft (func (param "s" string) (result u32)))
  (func $lifted (type $ft) (canon lift (core func $strlen) (memory $mem) (realloc $realloc)))
  (export "strlen" (func $lifted))
)
