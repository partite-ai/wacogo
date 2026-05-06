// Command wacogo-witgen generates Go bindings for a WIT file.
//
// Usage:
//
//   wacogo-witgen generate -w <world> -o <out-dir> -p <pkg-root> <wit-file>
//   wacogo-witgen --help
//   wacogo-witgen generate --help
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/partite-ai/wacogo/internal/witgen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "generate":
		cmdGenerate(os.Args[2:])
	case "--help", "-h", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "wacogo-witgen: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `wacogo-witgen — WIT-to-Go bindings generator

Usage:
    wacogo-witgen <command> [flags] [arguments]

Commands:
    generate    Generate Go bindings from a WIT file.
    help        Show this help.

Run "wacogo-witgen <command> --help" for command-specific help.`)
}

func cmdGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	var (
		world       = fs.String("world", "", "fully-qualified world id (required), e.g. foo:bar/world")
		worldShort  = fs.String("w", "", "short alias for --world")
		outDir      = fs.String("out", "", "output directory (required)")
		outDirShort = fs.String("o", "", "short alias for --out")
		pkgRoot     = fs.String("package-root", "", "Go module path prefix that maps to --out (required)")
		pkgRootShort = fs.String("p", "", "short alias for --package-root")
		dryRun      = fs.Bool("dry-run", false, "print files that would be written; no disk writes")
	)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `wacogo-witgen generate — generate Go bindings from a WIT file

Usage:
    wacogo-witgen generate -w <world> -o <out-dir> -p <pkg-root> <wit-file>

Required flags:
    -w, --world <id>           Fully-qualified world id (e.g. foo:bar/my-world)
    -o, --out <dir>            Output directory
    -p, --package-root <path>  Go module path prefix mapping to --out

Options:
    --dry-run                  Print files that would be written; no disk writes`)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flag details:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	// Resolve short/long aliases — pick the first non-empty.
	pick := func(short, long string) string {
		if long != "" {
			return long
		}
		return short
	}

	opts := witgen.Options{
		WitPath:     fs.Arg(0),
		World:       pick(*worldShort, *world),
		OutDir:      pick(*outDirShort, *outDir),
		PackageRoot: pick(*pkgRootShort, *pkgRoot),
		DryRun:      *dryRun,
	}

	files, err := witgen.Generate(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wacogo-witgen:", err)
		os.Exit(1)
	}
	if opts.DryRun {
		for path := range files {
			fmt.Println(path)
		}
	}
}
