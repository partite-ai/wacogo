package canon

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCallRawReportsResultMemoryBounds(t *testing.T) {
	for _, field := range []string{"pointer", "length"} {
		t.Run(field, func(t *testing.T) {
			mem := setupMem(t, 0)
			base := mem.Size()
			if field == "length" {
				base -= 4
				mem.WriteUint32Le(base, 0)
			}
			inst := &cleanupLiftInstance{table: newStubResourceTable()}
			binding := NewCallBinding(nil, []Type{testStringType{}}, Callee{
				CallSide: CallSide{Instance: inst, Memory: mem},
				Direct: func(_ context.Context, stack []uint64) {
					stack[0] = uint64(base)
				},
			})
			posted := 0
			binding.postReturn = func(context.Context, []uint64) error {
				posted++
				return nil
			}
			err := binding.CallRaw(t.Context(), func([]uint64) {}, func([]uint64) {
				t.Error("decoded an invalid result")
			})
			for _, part := range []string{"guest memory", "string " + field, fmt.Sprintf("%#x", mem.Size())} {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Fatalf("missing %q in memory-bounds diagnostic: %v", part, err)
				}
			}
			if posted != 1 || inst.entered {
				t.Fatalf("cleanup: post-return=%d entered=%t", posted, inst.entered)
			}
		})
	}
}
