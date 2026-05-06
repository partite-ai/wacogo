// Package testdata is a placeholder so `go generate ./testdata/...` runs the
// test-fixture pipeline:
//   - syncspec re-pulls the upstream spec mirror from the SHA in spec/UPSTREAM.txt
//     (only when -spec-dir is provided to syncspec; the directive below
//     intentionally omits it so day-to-day generate runs do not re-fetch).
//   - spectestgen compiles every .wast under spec/ and local/ into JSON +
//     binary fixtures under generated/, mirroring the source path.
//
// Re-pull upstream (rare): go run ./tools/syncspec
// Recompile fixtures:     go generate ./testdata
package testdata

//go:generate go run ../internal/spectestgen -manifest spec/skip.json -pair spec:generated/spec -pair local:generated/local
