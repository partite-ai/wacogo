//go:build ignore

// Fetch downloads the wasm-tools wasip1 build, extracts wasm-tools.wasm
// from the tarball, gzip-compresses it, and writes it next to this file
// as wasm-tools.wasm.gz. Run via `go generate ./internal/wasmtools/...`.
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	version = "1.248.0"
	urlTmpl = "https://github.com/bytecodealliance/wasm-tools/releases/download/v%s/wasm-tools-%s-wasm32-wasip1.tar.gz"
)

func main() {
	url := fmt.Sprintf(urlTmpl, version, version)
	fmt.Printf("fetching %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("download: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("download: HTTP %d", resp.StatusCode)
	}

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var binary []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("tar: %v", err)
		}
		if !strings.HasSuffix(hdr.Name, "/wasm-tools.wasm") {
			continue
		}
		binary, err = io.ReadAll(tr)
		if err != nil {
			log.Fatalf("read wasm-tools.wasm: %v", err)
		}
		break
	}
	if binary == nil {
		log.Fatal("wasm-tools.wasm not found in tarball")
	}
	fmt.Printf("extracted wasm-tools.wasm: %d bytes\n", len(binary))

	out, err := os.Create(filepath.Join(".", "wasm-tools.wasm.gz"))
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer out.Close()
	gw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		log.Fatalf("gzip writer: %v", err)
	}
	if _, err := gw.Write(binary); err != nil {
		log.Fatalf("write gzip: %v", err)
	}
	if err := gw.Close(); err != nil {
		log.Fatalf("close gzip: %v", err)
	}

	if info, err := out.Stat(); err == nil {
		fmt.Printf("wrote wasm-tools.wasm.gz: %d bytes\n", info.Size())
	}
}
