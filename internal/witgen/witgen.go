package witgen

import (
	"fmt"
	"os"
	"path/filepath"

	"go.bytecodealliance.org/wit"
)

// Options drives one Generate invocation.
type Options struct {
	// WitPath is the input WIT (text or JSON) file.
	WitPath string
	// World is the fully-qualified world id "ns:pkg/world".
	World string
	// OutDir is the root output directory; per-interface subdirs land here.
	OutDir string
	// PackageRoot is the Go module path prefix that maps to OutDir for
	// package declarations and import paths.
	PackageRoot string
	// DryRun, when true, returns the file map without writing.
	DryRun bool
}

// Generate executes one codegen invocation. Returns a map of relative
// path → file contents for the files that were (or would be) written.
// On DryRun==true, no disk writes occur.
func Generate(opts Options) (map[string][]byte, error) {
	if opts.WitPath == "" || opts.World == "" || opts.OutDir == "" || opts.PackageRoot == "" {
		return nil, fmt.Errorf("witgen: WitPath, World, OutDir, PackageRoot are all required")
	}
	res, err := Load(opts.WitPath)
	if err != nil {
		return nil, err
	}
	return generateFromResolve(res, opts)
}

func generateFromResolve(res *wit.Resolve, opts Options) (map[string][]byte, error) {
	pkg, err := LowerWorld(res, opts.World, opts.PackageRoot)
	if err != nil {
		return nil, err
	}
	if err := Validate(pkg); err != nil {
		return nil, fmt.Errorf("witgen: validation failed: %w", err)
	}
	sourceFile := filepath.Base(opts.WitPath)
	out := make(map[string][]byte, len(pkg.Interfaces)*2)
	for _, iface := range pkg.Interfaces {
		relDir := filepath.Join(iface.Namespace, iface.Package, iface.GoPackage)
		ifaceFile, err := emitIfaceFile(iface, sourceFile)
		if err != nil {
			return nil, fmt.Errorf("emit %s: %w", iface.Name, err)
		}
		bindFile, err := emitBindFile(iface, sourceFile)
		if err != nil {
			return nil, fmt.Errorf("emit %s.bind: %w", iface.Name, err)
		}
		wrapFile, err := emitWrapperFile(iface, sourceFile)
		if err != nil {
			return nil, fmt.Errorf("emit %s.wrap: %w", iface.Name, err)
		}
		out[filepath.Join(relDir, iface.GoPackage+".go")] = ifaceFile
		out[filepath.Join(relDir, iface.GoPackage+".bind.go")] = bindFile
		out[filepath.Join(relDir, iface.GoPackage+".wrap.go")] = wrapFile
	}
	if !opts.DryRun {
		for relPath, contents := range out {
			absPath := filepath.Join(opts.OutDir, relPath)
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(absPath), err)
			}
			if err := os.WriteFile(absPath, contents, 0o644); err != nil {
				return nil, fmt.Errorf("write %s: %w", absPath, err)
			}
		}
	}
	return out, nil
}
