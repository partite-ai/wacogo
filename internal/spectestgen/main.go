// spectestgen walks one or more directories of .wast files and produces
// matching JSON + .wasm fixtures via `wasm-tools json-from-wast`.
//
// For each input file IN/<rel>/<name>.wast it writes:
//   OUT/<rel>/<name>.json
//   OUT/<rel>/<name>/*.wasm
//
// Multiple -in/-out pairs may be supplied (repeat the flag); they are
// processed in order. The output directory for each pair is wiped before
// regeneration so deleted upstream files don't linger as stale fixtures.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/partite-ai/wacogo/internal/spectest"
)

type ioPair struct {
	in, out string
}

type pairsFlag []ioPair

func (p *pairsFlag) String() string { return fmt.Sprintf("%v", *p) }

func (p *pairsFlag) Set(s string) error {
	in, out, ok := strings.Cut(s, ":")
	if !ok {
		return fmt.Errorf("expected IN:OUT, got %q", s)
	}
	*p = append(*p, ioPair{in: in, out: out})
	return nil
}

func main() {
	var pairs pairsFlag
	flag.Var(&pairs, "pair", "IN:OUT directory pair (may be repeated)")
	manifestPath := flag.String("manifest", "", "optional path to a skip manifest; files marked as whole-file skips are not compiled (relative to each -pair IN)")
	flag.Parse()

	if len(pairs) == 0 {
		log.Fatal("at least one -pair IN:OUT is required")
	}
	if _, err := exec.LookPath("wasm-tools"); err != nil {
		log.Fatal("wasm-tools not found in PATH (requires >= 1.245.0)")
	}

	var manifest *spectest.SkipManifest
	if *manifestPath != "" {
		m, err := spectest.LoadSkipManifest(*manifestPath)
		if err != nil {
			log.Fatalf("load manifest: %v", err)
		}
		manifest = m
	}

	for _, p := range pairs {
		if err := regenerate(p.in, p.out, manifest); err != nil {
			log.Fatalf("%s -> %s: %v", p.in, p.out, err)
		}
	}
	fmt.Println("done")
}

func regenerate(in, out string, manifest *spectest.SkipManifest) error {
	if err := os.RemoveAll(out); err != nil {
		return fmt.Errorf("clear out dir: %w", err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	return filepath.WalkDir(in, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".wast") {
			return nil
		}
		rel, err := filepath.Rel(in, path)
		if err != nil {
			return err
		}
		if manifest != nil {
			if reason := manifest.FileSkip(filepath.ToSlash(rel)); reason != "" {
				fmt.Printf("skipping %s (%s)\n", rel, reason)
				return nil
			}
		}
		stem := strings.TrimSuffix(rel, ".wast")
		jsonPath := filepath.Join(out, stem+".json")
		wasmDir := filepath.Join(out, stem)
		if err := os.MkdirAll(wasmDir, 0o755); err != nil {
			return fmt.Errorf("create wasm dir for %s: %w", stem, err)
		}

		fmt.Printf("generating %s...\n", rel)
		cmd := exec.Command("wasm-tools", "json-from-wast",
			"--pretty",
			"--wasm-dir", wasmDir,
			"-o", jsonPath,
			path,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("wasm-tools json-from-wast failed for %s: %w", rel, err)
		}
		return nil
	})
}
