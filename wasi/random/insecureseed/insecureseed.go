// Package insecureseed implements the wasi:random/insecure-seed host
// component. The interface returns a 128-bit value used by guest languages
// to seed hash-map DoS protection; we source the bytes from crypto/rand.
package insecureseed

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/random/insecureseed"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	InsecureSeed = gen.InsecureSeed
	TupleU64U64  = gen.TupleU64U64
)

type seeder struct {
	src io.Reader
}

func newSeeder() *seeder {
	return &seeder{src: cryptorand.Reader}
}

func (s *seeder) InsecureSeed(_ context.Context) (TupleU64U64, error) {
	var buf [16]byte
	if _, err := io.ReadFull(s.src, buf[:]); err != nil {
		return TupleU64U64{}, fmt.Errorf("wasi:random/insecure-seed.insecure-seed: %w", err)
	}
	return TupleU64U64{
		F0: binary.LittleEndian.Uint64(buf[0:8]),
		F1: binary.LittleEndian.Uint64(buf[8:16]),
	}, nil
}

var _ gen.InsecureSeed = (*seeder)(nil)

func NewInstance(ctx context.Context, engine *wacogo.Engine, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, newSeeder(), nil, opts...)
}
