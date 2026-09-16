package micrographrag

import (
	"context"
	"fmt"

	potion "github.com/trengrj/go-potion"
)

type PotionEmbedder struct {
	model *potion.Potion
}

// NewPotionEmbedder loads potion-base-2M (64 dimensions). If GO_POTION_HOME
// already contains BASE2M/model.safetensors and BASE2M/tokenizer.json, no
// network access is performed. This is the recommended deployment mode.
func NewPotionEmbedder(ctx context.Context) (*PotionEmbedder, error) {
	p, err := potion.New(ctx, potion.BASE2M)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: load potion embedder: %w", err)
	}
	return &PotionEmbedder{model: p}, nil
}

func (p *PotionEmbedder) Dimensions() int { return VectorDimensions }

func (p *PotionEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, err := p.model.Encode(text)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: potion encode: %w", err)
	}
	if len(v) != VectorDimensions {
		return nil, fmt.Errorf("micrographrag: embedder returned %d dimensions, want %d", len(v), VectorDimensions)
	}
	return v, nil
}

func (p *PotionEmbedder) Close() error {
	if p == nil || p.model == nil {
		return nil
	}
	return p.model.Close()
}
