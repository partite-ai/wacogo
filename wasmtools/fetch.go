//go:build ignore

// Fetch downloads the wasm-tools wasip1 build, verifies its SHA-256 against
// a pinned digest, extracts wasm-tools.wasm from the tarball, gzip-compresses
// it, and writes it next to this file as wasm-tools.wasm.gz. Run via
// `go generate ./wasmtools/...`.
//
// To update to a new wasm-tools version: bump `version`, run fetch with
// `tarballSHA256 = ""` (the script will print the observed digest and exit
// non-zero so a new pin must be set explicitly).
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
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
	// tarballSHA256 is the SHA-256 of wasm-tools-<version>-wasm32-wasip1.tar.gz
	// (the gzipped archive as served by GitHub releases). When bumping
	// `version`, set this to "" first to print the new observed digest, then
	// re-run with the pinned value. See UPSTREAM.txt.
	tarballSHA256 = ""

	// maxTarballSize bounds the download to prevent decompression-bomb /
	// runaway-download conditions. wasm-tools-1.248.0 is ~6.5 MB gzipped.
	maxTarballSize = 100 * 1024 * 1024 // 100 MiB

	// maxBinarySize bounds the uncompressed wasm-tools.wasm extraction.
	maxBinarySize = 200 * 1024 * 1024 // 200 MiB
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

	tarballBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxTarballSize+1))
	if err != nil {
		log.Fatalf("download read: %v", err)
	}
	if len(tarballBytes) > maxTarballSize {
		log.Fatalf("download: tarball exceeds %d bytes", maxTarballSize)
	}

	gotDigest := sha256.Sum256(tarballBytes)
	gotHex := hex.EncodeToString(gotDigest[:])
	if tarballSHA256 == "" {
		log.Fatalf("tarball SHA-256 = %s; set tarballSHA256 = %q to pin and re-run", gotHex, gotHex)
	}
	if !strings.EqualFold(gotHex, tarballSHA256) {
		log.Fatalf("tarball SHA-256 mismatch: got %s, expected %s", gotHex, tarballSHA256)
	}
	fmt.Printf("verified tarball SHA-256 = %s\n", gotHex)

	gr, err := gzip.NewReader(strings.NewReader(string(tarballBytes)))
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
		// Reject path-traversal entry names defensively.
		if strings.Contains(hdr.Name, "..") {
			log.Fatalf("tar: refusing entry with parent-directory component: %q", hdr.Name)
		}
		binary, err = io.ReadAll(io.LimitReader(tr, maxBinarySize+1))
		if err != nil {
			log.Fatalf("read wasm-tools.wasm: %v", err)
		}
		if len(binary) > maxBinarySize {
			log.Fatalf("wasm-tools.wasm exceeds %d bytes", maxBinarySize)
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
