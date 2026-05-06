// Package random implements the wasi:random/random host component
// backed by Go's crypto/rand.
package random

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/random/random"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Random = gen.Random
)

type rng struct {
	src io.Reader
}

func newRNG() *rng {
	return &rng{src: cryptorand.Reader}
}

func (r *rng) GetRandomBytes(_ context.Context, n uint64) ([]uint8, error) {
	if n == 0 {
		return []uint8{}, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r.src, buf); err != nil {
		return nil, fmt.Errorf("wasi:random/random.get-random-bytes: %w", err)
	}
	return buf, nil
}

func (r *rng) GetRandomU64(_ context.Context) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r.src, buf[:]); err != nil {
		return 0, fmt.Errorf("wasi:random/random.get-random-u64: %w", err)
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

var _ gen.Random = (*rng)(nil)

func NewInstance(ctx context.Context, engine *wacogo.Engine) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, newRNG(), nil)
}
