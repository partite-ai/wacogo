// syncspec downloads the WebAssembly/component-model `test/` tree at the
// commit pinned in testdata/spec/UPSTREAM.txt and replaces the contents of
// testdata/spec/ with that tree. Intended to be run from the repo root.
package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	specDir := flag.String("spec-dir", "testdata/spec", "directory holding UPSTREAM.txt and the mirror")
	flag.Parse()

	commit, err := readCommit(filepath.Join(*specDir, "UPSTREAM.txt"))
	if err != nil {
		fail("read UPSTREAM.txt: %v", err)
	}

	url := fmt.Sprintf("https://codeload.github.com/WebAssembly/component-model/tar.gz/%s", commit)
	fmt.Fprintf(os.Stderr, "fetching %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		fail("download: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fail("download: HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		fail("gunzip: %v", err)
	}
	defer gz.Close()

	if err := clearMirror(*specDir); err != nil {
		fail("clear mirror: %v", err)
	}

	tr := tar.NewReader(gz)
	prefix := fmt.Sprintf("component-model-%s/test/", commit)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail("tar: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(hdr.Name, prefix)
		if !strings.HasSuffix(rel, ".wast") {
			continue
		}
		dst := filepath.Join(*specDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			fail("mkdir %s: %v", filepath.Dir(dst), err)
		}
		f, err := os.Create(dst)
		if err != nil {
			fail("create %s: %v", dst, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			fail("write %s: %v", dst, err)
		}
		f.Close()
		count++
	}
	fmt.Fprintf(os.Stderr, "wrote %d .wast files into %s\n", count, *specDir)
}

func readCommit(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "commit" {
			return strings.TrimSpace(v), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no commit: line in %s", path)
}

func clearMirror(specDir string) error {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "UPSTREAM.txt" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(specDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "syncspec: "+format+"\n", args...)
	os.Exit(1)
}
