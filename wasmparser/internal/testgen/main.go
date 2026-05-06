package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/partite-ai/wacogo/wasmtools"
)

func main() {
	wastDir := flag.String("wast-dir", "", "directory containing .wast files")
	outDir := flag.String("out-dir", "", "output directory for generated JSON and wasm files")
	flag.Parse()

	if *wastDir == "" || *outDir == "" {
		log.Fatal("both -wast-dir and -out-dir are required")
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("creating output dir: %v", err)
	}

	ctx := context.Background()
	tool, err := wasmtools.New(ctx)
	if err != nil {
		log.Fatalf("wasmtools.New: %v", err)
	}
	defer tool.Close(ctx)

	entries, err := os.ReadDir(*wastDir)
	if err != nil {
		log.Fatalf("reading wast dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wast") {
			continue
		}
		stem := strings.TrimSuffix(entry.Name(), ".wast")
		fmt.Printf("generating %s...\n", stem)
		if err := tool.JsonFromWast(ctx, filepath.Join(*wastDir, entry.Name()), *outDir); err != nil {
			log.Fatalf("wasm-tools json-from-wast failed for %s: %v", stem, err)
		}
	}
	fmt.Println("done")
}
