// Calculator example: implements a WIT interface in Go, wires it into
// a wasm component, calls add through the wasm boundary.
//
// Run with: go run github.com/partite-ai/wacogo/examples/calculator
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/examples/calculator/gen/example/demo/calc"
	"github.com/partite-ai/wacogo/wasmtools"
)

//go:generate go run github.com/partite-ai/wacogo/cmd/wacogo-witgen generate -w example:demo/arith -o ./gen -p github.com/partite-ai/wacogo/examples/calculator/gen ./calculator.wit

type myCalc struct{}

func (myCalc) Add(ctx context.Context, a, b uint32) (uint32, error)      { return a + b, nil }
func (myCalc) Subtract(ctx context.Context, a, b uint32) (uint32, error) { return a - b, nil }
func (myCalc) Multiply(ctx context.Context, a, b uint32) (uint32, error) { return a * b, nil }

func main() {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := calc.NewFactory(ctx, e)
	if err != nil {
		log.Fatal(err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myCalc{}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer hostInst.Close(ctx)

	// Tiny wasm importer that re-exports add.
	importerWAT := `(component
  (type $ft (func (param "a" u32) (param "b" u32) (result u32)))
  (import "h" (instance $h
    (export "add" (func (type $ft)))
  ))
  (alias export $h "add" (func $a))
  (export "add" (func $a))
)`
	bin := watToBinary(ctx, importerWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		log.Fatal(err)
	}
	inst, err := comp.Instantiate(ctx, wacogo.WithInstanceImport("h", hostInst.Core()))
	if err != nil {
		log.Fatal(err)
	}
	defer inst.Close(ctx)

	fn := inst.ExportedFunc("add")
	got, err := fn.Call(ctx, wacogo.ValU32(7), wacogo.ValU32(35))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("7 + 35 = %d\n", uint32(got[0].(wacogo.ValU32)))
}

func watToBinary(ctx context.Context, wat string) []byte {
	tool, err := wasmtools.Default(ctx)
	if err != nil {
		log.Fatal(err)
	}
	out, err := tool.Parse(ctx, []byte(wat))
	if err != nil {
		log.Fatal(err)
	}
	return out
}
