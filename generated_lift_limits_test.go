package wacogo_test

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/primlists"
)

func TestGeneratedResultsRespectLiftLimits(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		limits                wacogo.LiftLimits
		ptr, length           uint32
		limitError, spanError bool
	}{
		{"reported bypass", wacogo.LiftLimits{MaxBytes: 32, MaxValues: 16}, 64, 64, true, false},
		{"byte limit", wacogo.LiftLimits{MaxBytes: 32}, 64, 64, true, false},
		{"value limit", wacogo.LiftLimits{MaxValues: 16}, 64, 64, true, false},
		{"boundary", wacogo.LiftLimits{MaxBytes: 1024, MaxValues: 65}, 64, 64, false, false},
		{"unlimited", wacogo.LiftLimits{}, 64, 64, false, false},
		{"span", wacogo.LiftLimits{}, 65535, 64, false, true},
		{"hostile length", wacogo.LiftLimits{MaxValues: 16}, 64, 1 << 30, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			engine := wacogo.NewEngine(ctx, wacogo.WithLiftLimits(tc.limits))
			defer engine.Close(ctx)
			wat := fmt.Sprintf(`(component
  (core module $m
    (memory (export "memory") 1)
    (global $posts (mut i32) (i32.const 0))
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 1024)
    (func (export "run") (param i32 i32) (result i32)
      i32.const 0 i32.const %d i32.store
      i32.const 4 i32.const %d i32.store
      i32.const 0)
    (func (export "post") (param i32)
      global.get $posts i32.const 1 i32.add global.set $posts)
    (func (export "posts") (result i32) global.get $posts))
  (core instance $i (instantiate $m))
  (func (export "rev-u8") (param "xs" (list u8)) (result (list u8))
    (canon lift (core func $i "run") (memory (core memory $i "memory"))
      (realloc (core func $i "realloc")) (post-return (core func $i "post"))))
  (func (export "posts") (result u32) (canon lift (core func $i "posts"))))`, tc.ptr, tc.length)
			cmd := exec.Command("wasm-tools", "parse", "-")
			cmd.Stdin = strings.NewReader(wat)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			binary, err := cmd.Output()
			if err != nil {
				t.Fatalf("parse: %v: %s", err, stderr.String())
			}
			component, err := engine.LoadComponent(ctx, bytes.NewReader(binary))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close(ctx)
			callerComponent, err := engine.NewHostBuilder("caller").Build(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer callerComponent.Close(ctx)
			caller, err := callerComponent.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer caller.Close(ctx)
			wrapped := primlists.WrapInstance(caller, instance)
			for attempt := 1; attempt <= 2; attempt++ {
				got, err := wrapped.RevU8(ctx, nil)
				if tc.limitError {
					if !errors.Is(err, wacogo.ErrLiftLimit) || got != nil {
						t.Fatalf("generated decoder bypass: length=%d err=%v", len(got), err)
					}
				} else if tc.spanError {
					if err == nil || got != nil {
						t.Fatalf("bad span decoded: length=%d err=%v", len(got), err)
					}
				} else if err != nil || len(got) != int(tc.length) {
					t.Fatalf("valid decode: length=%d err=%v", len(got), err)
				}
				posts, err := instance.ExportedFunc("posts").Call(ctx)
				if err != nil || len(posts) != 1 || posts[0] != wacogo.ValU32(attempt) {
					t.Fatalf("post-return: values=%v err=%v", posts, err)
				}
			}
		})
	}
}
