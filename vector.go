package micrographrag

import (
	"fmt"
	"math"
)

func QuantizeI8(v []float32) ([]byte, error) {
	if len(v) != VectorDimensions {
		return nil, fmt.Errorf("micrographrag: quantize: got %d dimensions, want %d", len(v), VectorDimensions)
	}
	var maxAbs float32
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return nil, fmt.Errorf("micrographrag: quantize: non-finite value")
		}
		a := float32(math.Abs(float64(x)))
		if a > maxAbs {
			maxAbs = a
		}
	}
	if maxAbs < 1e-8 {
		return nil, ErrZeroVector
	}
	out := make([]byte, len(v))
	scale := float32(127.0) / maxAbs
	for i, x := range v {
		q := int(math.Round(float64(x * scale)))
		if q > 127 {
			q = 127
		}
		if q < -127 {
			q = -127
		}
		out[i] = byte(int8(q))
	}
	return out, nil
}
