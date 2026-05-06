// Usage: imports-exports <file.wasm>
//
// Reads a WebAssembly component binary and prints its imports and exports,
// showing the name and type kind of each.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/partite-ai/wacogo/wasmparser"
)

// typeRefKind returns a human-readable kind name for a ComponentTypeRef.
func typeRefKind(t wasmparser.ComponentTypeRef) string {
	switch t.(type) {
	case wasmparser.TypeRefModule:
		return "module"
	case wasmparser.TypeRefFunc:
		return "func"
	case wasmparser.TypeRefValue:
		return "value"
	case wasmparser.TypeRefType:
		return "type"
	case wasmparser.TypeRefComponent:
		return "component"
	case wasmparser.TypeRefInstance:
		return "instance"
	default:
		return "unknown"
	}
}

// externalKindName returns a human-readable name for a ComponentExternalKind.
func externalKindName(k wasmparser.ComponentExternalKind) string {
	switch k {
	case wasmparser.ExternalKindModule:
		return "module"
	case wasmparser.ExternalKindFunc:
		return "func"
	case wasmparser.ExternalKindValue:
		return "value"
	case wasmparser.ExternalKindType:
		return "type"
	case wasmparser.ExternalKindComponent:
		return "component"
	case wasmparser.ExternalKindInstance:
		return "instance"
	default:
		return "unknown"
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: imports-exports <file.wasm>\n")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	printedImports := false
	printedExports := false

	for payload, err := range wasmparser.ParseAll(f) {
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		switch p := payload.(type) {
		case *wasmparser.ComponentImportSectionPayload:
			if !printedImports {
				fmt.Println("imports:")
				printedImports = true
			}
			for imp, err := range p.Items() {
				if err != nil {
					log.Fatalf("error reading import: %v", err)
				}
				fmt.Printf("  %q (%s)\n", imp.Name.Name, typeRefKind(imp.Type))
			}

		case *wasmparser.ComponentExportSectionPayload:
			if !printedExports {
				fmt.Println("exports:")
				printedExports = true
			}
			for exp, err := range p.Items() {
				if err != nil {
					log.Fatalf("error reading export: %v", err)
				}
				fmt.Printf("  %q (%s)\n", exp.Name.Name, externalKindName(exp.Kind))
			}
		}
	}

	if !printedImports {
		fmt.Println("imports: (none)")
	}
	if !printedExports {
		fmt.Println("exports: (none)")
	}
}
