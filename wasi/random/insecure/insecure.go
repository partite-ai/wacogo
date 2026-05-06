// Package insecure implements the wasi:random/insecure host component.
// The interface is documented as "insecure pseudo-random"; we back it with
// crypto/rand which trivially satisfies the spec's distribution and period
// guidance without exposing a separately-seeded PRNG.
package insecure

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/random/insecure"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Insecure = gen.Insecure
)

type rng struct {
	src io.Reader
}

func newRNG() *rng {
	return &rng{src: cryptorand.Reader}
}

func (r *rng) GetInsecureRandomBytes(_ context.Context, n uint64) ([]uint8, error) {
	if n == 0 {
		return []uint8{}, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r.src, buf); err != nil {
		return nil, fmt.Errorf("wasi:random/insecure.get-insecure-random-bytes: %w", err)
	}
	return buf, nil
}

func (r *rng) GetInsecureRandomU64(_ context.Context) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r.src, buf[:]); err != nil {
		return 0, fmt.Errorf("wasi:random/insecure.get-insecure-random-u64: %w", err)
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

var _ gen.Insecure = (*rng)(nil)

func NewInstance(ctx context.Context, engine *wacogo.Engine) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, newRNG(), nil)
}
