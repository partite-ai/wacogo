package types

import (
	"context"
	"maps"
	"net/http"

	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
)

type fieldsImpl struct {
	h         http.Header
	immutable bool
}

func newFieldsImpl(h http.Header, immutable bool) *fieldsImpl {
	return &fieldsImpl{h: h, immutable: immutable}
}

func (hf *fieldsImpl) Append(ctx context.Context, name string, value []uint8) (Result_HeaderError, error) {
	if hf.immutable {
		return Result_HeaderErrorErr{
			Value: types.HeaderErrorImmutable{},
		}, nil
	}
	hf.h.Add(name, string(value))
	return types.Result_HeaderErrorOk{}, nil
}

func (hf *fieldsImpl) Clone(ctx context.Context) (*FieldsHandle, error) {
	newHeaders := make(http.Header)
	maps.Copy(newHeaders, hf.h)
	return types.NewFieldsHandle(&fieldsImpl{newHeaders, false}), nil
}

func (hf *fieldsImpl) Delete(ctx context.Context, name string) (Result_HeaderError, error) {
	if hf.immutable {
		return Result_HeaderErrorErr{
			Value: types.HeaderErrorImmutable{},
		}, nil
	}
	hf.h.Del(name)
	return types.Result_HeaderErrorOk{}, nil
}

func (hf *fieldsImpl) Entries(ctx context.Context) ([]TupleStringListU8, error) {
	entries := make([]TupleStringListU8, 0, len(hf.h))
	for name, values := range hf.h {
		for _, v := range values {
			entries = append(entries, TupleStringListU8{F0: name, F1: []uint8(v)})
		}
	}
	return entries, nil
}

func (hf *fieldsImpl) Get(ctx context.Context, name string) ([][]uint8, error) {
	values := hf.h.Values(name)
	result := make([][]uint8, len(values))
	for i, v := range values {
		result[i] = []uint8(v)
	}
	return result, nil
}

func (hf *fieldsImpl) Has(ctx context.Context, name string) (bool, error) {
	return len(hf.h.Values(name)) > 0, nil
}

func (hf *fieldsImpl) Set(ctx context.Context, name string, value [][]uint8) (Result_HeaderError, error) {
	if hf.immutable {
		return Result_HeaderErrorErr{
			Value: types.HeaderErrorImmutable{},
		}, nil
	}
	hf.h.Del(name)
	for _, v := range value {
		hf.h.Add(name, string(v))
	}
	return types.Result_HeaderErrorOk{}, nil
}

func headersFromFields(ctx context.Context, fh *FieldsHandle) (http.Header, error) {
	if impl, ok := fh.LocalImpl(); ok {
		if fieldsImpl, ok := impl.(*fieldsImpl); ok {
			return fieldsImpl.h, nil
		}
	}

	h := make(http.Header)
	entries, err := fh.Entries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		for _, v := range entry.F1 {
			h.Add(entry.F0, string(v))
		}
	}
	return h, nil
}
