// wasi-greet loads a wasi:cli@0.2.8 component from disk, wires up the
// full wasi:cli/imports world (stdio plumbed to this process), and
// invokes an exported func(name: string) -> string.
//
// Usage:
//
//	wasi-greet <component.wasm> <name> [export]
//
// The third argument selects the export to call:
//   - "example:people/greet#greeting" (default)
//   - "iface/path#fn"  — function "fn" inside the exported instance
//     named "iface/path" (e.g. "example:people/greet@0.1.0#greeting").
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("usage: %s <component.wasm> <name> [export]", os.Args[0])
	}
	wasmPath := os.Args[1]
	name := os.Args[2]
	exportSpec := "example:people/greet#greeting"
	if len(os.Args) > 3 {
		exportSpec = os.Args[3]
	}

	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	w, err := wasi.NewWorld(ctx, e, &wasi.Config{
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		log.Fatalf("build wasi world: %v", err)
	}
	defer w.Close(ctx)

	hb := e.NewHostBuilder("example:people/people")
	personType := hb.AddResource("person", nil)
	hb.AddFunction("[method]person.get-name", &host.FuncType{
		Params: []host.Param{
			{
				Name: "self",
				Type: personType.Borrow(),
			},
		},
		Results: []host.ResultDecl{
			{
				Type: host.String,
			},
		},
	}, func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64) error {
		name := "Alice"
		ptr, _ := cc.Realloc(ctx, 0, 0, 1, uint32(len(name)))
		cc.Memory().Write(ptr, []byte(name))
		stack[0] = uint64(ptr)
		stack[1] = uint64(len(name))
		return nil
	})

	peopleComp, err := hb.Build(ctx)
	if err != nil {
		log.Fatalf("build people component: %v", err)
	}
	defer peopleComp.Close(ctx)

	peopleInst, err := peopleComp.Instantiate(ctx)
	if err != nil {
		log.Fatalf("instantiate people component: %v", err)
	}
	defer peopleInst.Close(ctx)

	f, err := os.Open(wasmPath)
	if err != nil {
		log.Fatalf("open %s: %v", wasmPath, err)
	}
	defer f.Close()

	comp, err := e.LoadComponent(ctx, f)
	if err != nil {
		log.Fatalf("load component: %v", err)
	}

	opts := []wacogo.InstantiateOption{
		wacogo.WithInstanceImport("example:people/people", peopleInst.Core()),
	}
	opts = append(opts, w.Imports()...)

	inst, err := comp.Instantiate(ctx, opts...)
	if err != nil {
		log.Fatalf("instantiate: %v", err)
	}
	defer inst.Close(ctx)

	fn, err := resolveExport(inst, exportSpec)
	if err != nil {
		log.Fatal(err)
	}

	got, err := fn.Call(ctx, wacogo.ValString(name))
	if err != nil {
		log.Fatalf("call %s: %v", exportSpec, err)
	}
	if len(got) != 1 {
		log.Fatalf("expected 1 result from %s, got %d", exportSpec, len(got))
	}
	s, ok := got[0].(wacogo.ValString)
	if !ok {
		log.Fatalf("expected string result from %s, got %T", exportSpec, got[0])
	}
	fmt.Println(string(s))
}

// resolveExport finds a top-level or nested-instance exported function
// from spec. spec is "fn" for top-level or "iface#fn" for nested.
func resolveExport(inst *wacogo.ComponentInstance, spec string) (*wacogo.ExportedFunc, error) {
	ifaceName, fnName, nested := strings.Cut(spec, "#")
	if !nested {
		fn := inst.ExportedFunc(spec)
		if fn == nil {
			return nil, fmt.Errorf("no exported func %q on component", spec)
		}
		return fn, nil
	}
	sub := inst.ExportedInstance(ifaceName)
	if sub == nil {
		return nil, fmt.Errorf("no exported instance %q on component", ifaceName)
	}
	fn := sub.ExportedFunc(fnName)
	if fn == nil {
		return nil, fmt.Errorf("no exported func %q on instance %q", fnName, ifaceName)
	}
	return fn, nil
}
