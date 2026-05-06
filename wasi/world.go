package wasi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"

	"github.com/partite-ai/wacogo/wasi/cli/environment"
	"github.com/partite-ai/wacogo/wasi/cli/exit"
	"github.com/partite-ai/wacogo/wasi/cli/stderr"
	"github.com/partite-ai/wacogo/wasi/cli/stdin"
	"github.com/partite-ai/wacogo/wasi/cli/stdout"
	"github.com/partite-ai/wacogo/wasi/cli/terminalinput"
	"github.com/partite-ai/wacogo/wasi/cli/terminaloutput"
	"github.com/partite-ai/wacogo/wasi/cli/terminalstderr"
	"github.com/partite-ai/wacogo/wasi/cli/terminalstdin"
	"github.com/partite-ai/wacogo/wasi/cli/terminalstdout"
	monotonicclock "github.com/partite-ai/wacogo/wasi/clocks/monotonicclock"
	timezone "github.com/partite-ai/wacogo/wasi/clocks/timezone"
	wallclock "github.com/partite-ai/wacogo/wasi/clocks/wallclock"
	fspreopens "github.com/partite-ai/wacogo/wasi/filesystem/preopens"
	fstypes "github.com/partite-ai/wacogo/wasi/filesystem/types"
	httpoutgoing "github.com/partite-ai/wacogo/wasi/http/outgoinghandler"
	httptypes "github.com/partite-ai/wacogo/wasi/http/types"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/io/streams"
	"github.com/partite-ai/wacogo/wasi/io/wioerror"
	randominsecure "github.com/partite-ai/wacogo/wasi/random/insecure"
	randominsecureseed "github.com/partite-ai/wacogo/wasi/random/insecureseed"
	randomrandom "github.com/partite-ai/wacogo/wasi/random/random"
	socketsinstancenetwork "github.com/partite-ai/wacogo/wasi/sockets/instancenetwork"
	socketsipnamelookup "github.com/partite-ai/wacogo/wasi/sockets/ipnamelookup"
	socketsnetwork "github.com/partite-ai/wacogo/wasi/sockets/network"
	socketstcp "github.com/partite-ai/wacogo/wasi/sockets/tcp"
	socketstcpcreate "github.com/partite-ai/wacogo/wasi/sockets/tcpcreatesocket"
	socketsudp "github.com/partite-ai/wacogo/wasi/sockets/udp"
	socketsudpcreate "github.com/partite-ai/wacogo/wasi/sockets/udpcreatesocket"
)

// World holds every host component instance that makes up the
// wasi:cli/imports world plus wasi:http/types and
// wasi:http/outgoing-handler. Use NewWorld to construct and Close to
// release all instances.
type World struct {
	Error              *host.ComponentInstance
	Poll               *host.ComponentInstance
	WallClock          *host.ComponentInstance
	Random             *host.ComponentInstance
	Insecure           *host.ComponentInstance
	InsecureSeed       *host.ComponentInstance
	Environment        *host.ComponentInstance
	Exit               *host.ComponentInstance
	TerminalInput      *host.ComponentInstance
	TerminalOutput     *host.ComponentInstance
	Streams            *host.ComponentInstance
	MonotonicClock     *host.ComponentInstance
	Timezone           *host.ComponentInstance
	Network            *host.ComponentInstance
	InstanceNetwork    *host.ComponentInstance
	IPNameLookup       *host.ComponentInstance
	FilesystemTypes    *host.ComponentInstance
	Stdin              *host.ComponentInstance
	Stdout             *host.ComponentInstance
	Stderr             *host.ComponentInstance
	TerminalStdin      *host.ComponentInstance
	TerminalStdout     *host.ComponentInstance
	TerminalStderr     *host.ComponentInstance
	TCP                *host.ComponentInstance
	UDP                *host.ComponentInstance
	FilesystemPreopens *host.ComponentInstance
	TCPCreateSocket    *host.ComponentInstance
	UDPCreateSocket    *host.ComponentInstance
	HttpTypes          *host.ComponentInstance
	HttpOutgoing       *host.ComponentInstance

	closeOrder []*host.ComponentInstance
}

// NewWorld instantiates every host import in topological dependency
// order. On any failure the partially-built world is closed and the
// error is returned wrapped with the failing interface name.
func NewWorld(ctx context.Context, e *wacogo.Engine, cfg *Config) (*World, error) {
	httpClient := cfg.HttpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	w := &World{}
	type step struct {
		name string
		run  func() (*host.ComponentInstance, error)
		set  func(*host.ComponentInstance)
	}

	steps := []step{
		// Layer 0 — no deps
		{"io.error", func() (*host.ComponentInstance, error) {
			return wioerror.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.Error = i }},
		{"io.poll", func() (*host.ComponentInstance, error) {
			return poll.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.Poll = i }},
		{"clocks.wall-clock", func() (*host.ComponentInstance, error) {
			return wallclock.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.WallClock = i }},
		{"random.random", func() (*host.ComponentInstance, error) {
			return randomrandom.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.Random = i }},
		{"random.insecure", func() (*host.ComponentInstance, error) {
			return randominsecure.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.Insecure = i }},
		{"random.insecure-seed", func() (*host.ComponentInstance, error) {
			return randominsecureseed.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.InsecureSeed = i }},
		{"cli.environment", func() (*host.ComponentInstance, error) {
			return environment.NewInstance(ctx, e, cfg.Args, cfg.Env, cfg.InitialCwd)
		}, func(i *host.ComponentInstance) { w.Environment = i }},
		{"cli.exit", func() (*host.ComponentInstance, error) {
			return exit.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.Exit = i }},
		{"cli.terminal-input", func() (*host.ComponentInstance, error) {
			return terminalinput.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.TerminalInput = i }},
		{"cli.terminal-output", func() (*host.ComponentInstance, error) {
			return terminaloutput.NewInstance(ctx, e)
		}, func(i *host.ComponentInstance) { w.TerminalOutput = i }},

		// Layer 1 — deps on Layer 0
		{"io.streams", func() (*host.ComponentInstance, error) {
			return streams.NewInstance(ctx, e, w.Error, w.Poll)
		}, func(i *host.ComponentInstance) { w.Streams = i }},
		{"clocks.monotonic-clock", func() (*host.ComponentInstance, error) {
			return monotonicclock.NewInstance(ctx, e, w.Poll)
		}, func(i *host.ComponentInstance) { w.MonotonicClock = i }},
		{"clocks.timezone", func() (*host.ComponentInstance, error) {
			return timezone.NewInstance(ctx, e, w.WallClock)
		}, func(i *host.ComponentInstance) { w.Timezone = i }},
		{"sockets.network", func() (*host.ComponentInstance, error) {
			return socketsnetwork.NewInstance(ctx, e, w.Error)
		}, func(i *host.ComponentInstance) { w.Network = i }},

		// Layer 2 — deps on Layer 1
		{"sockets.instance-network", func() (*host.ComponentInstance, error) {
			return socketsinstancenetwork.NewInstance(ctx, e, w.Network)
		}, func(i *host.ComponentInstance) { w.InstanceNetwork = i }},
		{"sockets.ip-name-lookup", func() (*host.ComponentInstance, error) {
			return socketsipnamelookup.NewInstance(ctx, e, w.Network, w.Poll)
		}, func(i *host.ComponentInstance) { w.IPNameLookup = i }},
		{"filesystem.types", func() (*host.ComponentInstance, error) {
			return fstypes.NewInstance(ctx, e, w.Error, w.Poll, w.Streams, w.WallClock)
		}, func(i *host.ComponentInstance) { w.FilesystemTypes = i }},
		{"cli.stdin", func() (*host.ComponentInstance, error) {
			return stdin.NewInstance(ctx, e, w.Streams, w.Error, w.Poll, cfg.Stdin)
		}, func(i *host.ComponentInstance) { w.Stdin = i }},
		{"cli.stdout", func() (*host.ComponentInstance, error) {
			return stdout.NewInstance(ctx, e, w.Streams, w.Error, w.Poll, cfg.Stdout)
		}, func(i *host.ComponentInstance) { w.Stdout = i }},
		{"cli.stderr", func() (*host.ComponentInstance, error) {
			return stderr.NewInstance(ctx, e, w.Streams, w.Error, w.Poll, cfg.Stderr)
		}, func(i *host.ComponentInstance) { w.Stderr = i }},
		{"cli.terminal-stdin", func() (*host.ComponentInstance, error) {
			return terminalstdin.NewInstance(ctx, e, w.TerminalInput)
		}, func(i *host.ComponentInstance) { w.TerminalStdin = i }},
		{"cli.terminal-stdout", func() (*host.ComponentInstance, error) {
			return terminalstdout.NewInstance(ctx, e, w.TerminalOutput)
		}, func(i *host.ComponentInstance) { w.TerminalStdout = i }},
		{"cli.terminal-stderr", func() (*host.ComponentInstance, error) {
			return terminalstderr.NewInstance(ctx, e, w.TerminalOutput)
		}, func(i *host.ComponentInstance) { w.TerminalStderr = i }},
		{"sockets.tcp", func() (*host.ComponentInstance, error) {
			return socketstcp.NewInstance(ctx, e, w.Error, w.Network, w.Poll, w.Streams)
		}, func(i *host.ComponentInstance) { w.TCP = i }},
		{"sockets.udp", func() (*host.ComponentInstance, error) {
			return socketsudp.NewInstance(ctx, e, w.Network, w.Poll)
		}, func(i *host.ComponentInstance) { w.UDP = i }},
		{"http.types", func() (*host.ComponentInstance, error) {
			return httptypes.NewInstance(ctx, e, w.Error, w.Poll, w.Streams)
		}, func(i *host.ComponentInstance) { w.HttpTypes = i }},

		// Layer 3 — deps on Layer 2
		{"filesystem.preopens", func() (*host.ComponentInstance, error) {
			var impl fspreopens.Preopens = fspreopens.EmptyFS{}
			if cfg.Preopens != nil {
				impl = cfg.Preopens(fspreopens.Deps{
					Types:     w.FilesystemTypes,
					Streams:   w.Streams,
					Error:     w.Error,
					Poll:      w.Poll,
					WallClock: w.WallClock,
				})
			}
			return fspreopens.NewInstance(ctx, e, w.Error, w.Poll, w.Streams, w.FilesystemTypes, w.WallClock, impl)
		}, func(i *host.ComponentInstance) { w.FilesystemPreopens = i }},
		{"sockets.tcp-create-socket", func() (*host.ComponentInstance, error) {
			return socketstcpcreate.NewInstance(ctx, e, w.Error, w.Network, w.Poll, w.Streams, w.TCP)
		}, func(i *host.ComponentInstance) { w.TCPCreateSocket = i }},
		{"sockets.udp-create-socket", func() (*host.ComponentInstance, error) {
			return socketsudpcreate.NewInstance(ctx, e, w.Network, w.Poll, w.UDP)
		}, func(i *host.ComponentInstance) { w.UDPCreateSocket = i }},
		{"http.outgoing-handler", func() (*host.ComponentInstance, error) {
			return httpoutgoing.NewInstance(ctx, e, w.HttpTypes, w.Error, w.Poll, w.Streams, httpClient)
		}, func(i *host.ComponentInstance) { w.HttpOutgoing = i }},
	}

	for _, s := range steps {
		inst, err := s.run()
		if err != nil {
			_ = w.Close(ctx)
			return nil, fmt.Errorf("wasi: instantiate %s: %w", s.name, err)
		}
		s.set(inst)
		w.closeOrder = append(w.closeOrder, inst)
	}
	return w, nil
}

// Imports returns one wacogo.WithInstanceImport option per host instance
// in this World, keyed by each interface's canonical InterfaceName
// (e.g., "wasi:cli/environment@0.2.8"). Pass the returned slice into
// (*wacogo.Component).Instantiate when wiring a component that imports
// the wasi:cli/imports + wasi:http/imports worlds.
func (w *World) Imports() []wacogo.InstantiateOption {
	return []wacogo.InstantiateOption{
		wacogo.WithInstanceImport(wioerror.InterfaceName, w.Error.Core()),
		wacogo.WithInstanceImport(poll.InterfaceName, w.Poll.Core()),
		wacogo.WithInstanceImport(streams.InterfaceName, w.Streams.Core()),
		wacogo.WithInstanceImport(wallclock.InterfaceName, w.WallClock.Core()),
		wacogo.WithInstanceImport(monotonicclock.InterfaceName, w.MonotonicClock.Core()),
		wacogo.WithInstanceImport(timezone.InterfaceName, w.Timezone.Core()),
		wacogo.WithInstanceImport(randomrandom.InterfaceName, w.Random.Core()),
		wacogo.WithInstanceImport(randominsecure.InterfaceName, w.Insecure.Core()),
		wacogo.WithInstanceImport(randominsecureseed.InterfaceName, w.InsecureSeed.Core()),
		wacogo.WithInstanceImport(fstypes.InterfaceName, w.FilesystemTypes.Core()),
		wacogo.WithInstanceImport(fspreopens.InterfaceName, w.FilesystemPreopens.Core()),
		wacogo.WithInstanceImport(socketsnetwork.InterfaceName, w.Network.Core()),
		wacogo.WithInstanceImport(socketsinstancenetwork.InterfaceName, w.InstanceNetwork.Core()),
		wacogo.WithInstanceImport(socketsipnamelookup.InterfaceName, w.IPNameLookup.Core()),
		wacogo.WithInstanceImport(socketstcp.InterfaceName, w.TCP.Core()),
		wacogo.WithInstanceImport(socketsudp.InterfaceName, w.UDP.Core()),
		wacogo.WithInstanceImport(socketstcpcreate.InterfaceName, w.TCPCreateSocket.Core()),
		wacogo.WithInstanceImport(socketsudpcreate.InterfaceName, w.UDPCreateSocket.Core()),
		wacogo.WithInstanceImport(environment.InterfaceName, w.Environment.Core()),
		wacogo.WithInstanceImport(exit.InterfaceName, w.Exit.Core()),
		wacogo.WithInstanceImport(stdin.InterfaceName, w.Stdin.Core()),
		wacogo.WithInstanceImport(stdout.InterfaceName, w.Stdout.Core()),
		wacogo.WithInstanceImport(stderr.InterfaceName, w.Stderr.Core()),
		wacogo.WithInstanceImport(terminalinput.InterfaceName, w.TerminalInput.Core()),
		wacogo.WithInstanceImport(terminaloutput.InterfaceName, w.TerminalOutput.Core()),
		wacogo.WithInstanceImport(terminalstdin.InterfaceName, w.TerminalStdin.Core()),
		wacogo.WithInstanceImport(terminalstdout.InterfaceName, w.TerminalStdout.Core()),
		wacogo.WithInstanceImport(terminalstderr.InterfaceName, w.TerminalStderr.Core()),
		wacogo.WithInstanceImport(httptypes.InterfaceName, w.HttpTypes.Core()),
		wacogo.WithInstanceImport(httpoutgoing.InterfaceName, w.HttpOutgoing.Core()),
	}
}

// Close releases every component instance in reverse construction order,
// accumulating all errors via errors.Join.
func (w *World) Close(ctx context.Context) error {
	var errs []error
	for i := len(w.closeOrder) - 1; i >= 0; i-- {
		if err := w.closeOrder[i].Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	w.closeOrder = nil
	return errors.Join(errs...)
}
