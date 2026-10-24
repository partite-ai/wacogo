package canon

import (
	"context"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestHostGeneratedStorageRejectsBeforeTransfer(t *testing.T) {
	fields := make([]RecordField, 8)
	for i := range fields {
		fields[i].Type = testTypeU64{}
	}
	for _, tc := range []struct {
		name    string
		element Type
	}{
		{"empty nested lists", testTypeList{elem: testU8Type{}}},
		{"absent wide options", testTypeOption{inner: testTypeRecord{fields: fields}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			runtime := wazero.NewRuntime(ctx)
			defer runtime.Close(ctx)
			source, destination := setupMem(t, 0), setupMem(t, 0)
			params := []Type{testTypeList{elem: tc.element}}
			reallocs, calls := 0, 0
			adapter, err := NewHost(runtime).BuildAdapter(ctx, "bounded-generated-args",
				CallSide{Instance: &testInstance{}, Memory: source, StringEncoding: EncUTF8},
				Callee{
					CallSide: CallSide{Instance: &testInstance{}, Memory: destination, StringEncoding: EncUTF8,
						GoRealloc: func(context.Context, uint32, uint32, uint32, uint32) (uint32, error) { reallocs++; return 0, nil }},
					Arguments: NewLiftValidator(params, LiftLimits{MaxBytes: 32, MaxValues: 3}),
					Direct:    func(context.Context, []uint64) { calls++ },
				}, params, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close(ctx)
			_, err = adapter.Module.ExportedFunction(adapter.Name).Call(ctx, 128, 2)
			if err == nil || !strings.Contains(err.Error(), ErrLiftLimit.Error()) || reallocs != 0 || calls != 0 {
				t.Fatalf("transfer started before budget rejection: reallocs=%d calls=%d err=%v", reallocs, calls, err)
			}
		})
	}
}
