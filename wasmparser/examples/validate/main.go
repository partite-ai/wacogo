// Usage: validate <file.wasm>
//
// Reads a WebAssembly component binary and validates it using all features.
// Exits 0 and prints "valid component" on success, or exits 1 with the error.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/partite-ai/wacogo/wasmparser"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: validate <file.wasm>\n")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	vp := wasmparser.NewValidatingParser(f, wasmparser.AllFeatures())
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("valid component")
}
