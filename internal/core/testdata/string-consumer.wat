(component
  (type $strlen-type (func (param "s" string) (result u32)))
  (import "strlen" (func $imported-strlen (type $strlen-type)))

  ;; Memory module — instantiated first so canon lower can reference its memory.
  (core module $mem_mod
    (memory (export "memory") 1)
    (global $bump (mut i32) (i32.const 1024))
    (func (export "cabi_realloc") (param i32 i32 i32 i32) (result i32)
      (local $ptr i32)
      (local.set $ptr (global.get $bump))
      (local.set $ptr
        (i32.and
          (i32.add (local.get $ptr) (i32.sub (local.get 2) (i32.const 1)))
          (i32.sub (i32.const 0) (local.get 2))))
      (global.set $bump (i32.add (local.get $ptr) (local.get 3)))
      (local.get $ptr)
    )
  )

  ;; Step 1: Instantiate memory module
  (core instance $mem_inst (instantiate $mem_mod))
  (alias core export $mem_inst "memory" (core memory $consumer_mem))
  (alias core export $mem_inst "cabi_realloc" (core func $consumer_realloc))

  ;; Step 2: Lower the imported function with consumer's memory and realloc
  (core func $lowered-strlen (canon lower (func $imported-strlen)
    (memory $consumer_mem) (realloc $consumer_realloc)))

  ;; Step 3: Main module imports memory and the lowered strlen
  (core module $main
    (import "env" "memory" (memory 1))
    (import "env" "strlen" (func $strlen (param i32 i32) (result i32)))

    ;; measure: calls imported strlen, returns its result
    (func (export "measure") (param i32 i32) (result i32)
      local.get 0
      local.get 1
      call $strlen
    )
  )

  ;; Build the import instance for $main
  (core instance $env (export "memory" (memory $consumer_mem))
                      (export "strlen" (func $lowered-strlen)))
  (core instance $main_inst (instantiate $main (with "env" (instance $env))))

  (alias core export $main_inst "measure" (core func $measure))

  (type $measure-type (func (param "s" string) (result u32)))
  (func $lifted-measure (type $measure-type) (canon lift (core func $measure)
    (memory $consumer_mem) (realloc $consumer_realloc)))
  (export "measure" (func $lifted-measure))
)
