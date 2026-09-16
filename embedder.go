package micrographrag

import "context"

type Embedder interface {
	Dimensions() int
	Encode(ctx context.Context, text string) ([]float32, error)
	Close() error
}

type NoopEmbedder struct{}

func (NoopEmbedder) Dimensions() int { return 0 }
func (NoopEmbedder) Encode(context.Context, string) ([]float32, error) {
	return nil, ErrVectorDisabled
}
func (NoopEmbedder) Close() error { return nil }
