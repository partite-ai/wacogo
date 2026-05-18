package wasi

import (
	"io"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi/filesystem/preopens"
	"github.com/partite-ai/wacogo/wasi/http/types"
)

// HTTPDoer is the minimal interface wasi:http/outgoing-handler uses to
// issue outgoing requests. It is an alias for types.HTTPDoer; *http.Client
// satisfies it.
type HTTPDoer = types.HTTPDoer

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

	// HttpClient executes outgoing HTTP requests for
	// wasi:http/outgoing-handler. When nil, http.DefaultClient is used.
	//
	// Implementations may modify *http.Request before issuing it (URL,
	// headers, body) or deny it by returning an error. To surface a
	// specific wasi:http/types ErrorCode to the guest, return a
	// *types.CodedError (from wasi/http/types); any other error is
	// mapped via the existing translation, falling back to
	// ErrorCodeInternalError.
	HttpClient HTTPDoer

	// CallListener, if non-nil, is attached to every host component
	// instance that NewWorld creates. See host.CallListener.
	CallListener host.CallListener
}
