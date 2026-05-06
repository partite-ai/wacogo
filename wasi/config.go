package wasi

import (
	"io"
	"net/http"

	"github.com/partite-ai/wacogo/wasi/filesystem/preopens"
)

// Config configures NewWorld.
type Config struct {
	Args       []string
	Env        [][2]string
	InitialCwd string

	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// Preopens, if non-nil, is invoked after the host instances it
	// depends on are constructed. It returns the Preopens
	// implementation to bind to the wasi:filesystem/preopens host
	// component. When nil, an empty preopen set is exposed.
	Preopens func(preopens.Deps) preopens.Preopens

	// HttpClient is the *http.Client used by wasi:http/outgoing-handler.
	// When nil, http.DefaultClient is used.
	HttpClient *http.Client
}
