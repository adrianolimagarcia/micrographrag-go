package micrographrag

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Dimensions() int { return VectorDimensions }
func (fakeEmbedder) Close() error    { return nil }
func (fakeEmbedder) Encode(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, VectorDimensions)
	for i := range v {
		v[i] = float32((i%7)+1) / 7
	}
	if text == "weather" {
		for i := range v {
			v[i] = -v[i]
		}
	}
	return v, nil
}

func openTestStore(t *testing.T, withVector bool) *Store {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "test.db"))
	cfg.EnableWAL = false
	cfg.EnableEmbeddingWorker = false
	var emb Embedder
	if withVector {
		emb = fakeEmbedder{}
	} else {
		cfg.EnableVector = false
	}
	s, err := Open(context.Background(), cfg, emb)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestKV(t *testing.T) {
	s := openTestStore(t, false)
	ctx := context.Background()
	if err := s.PutKV(ctx, "agent", "state", []byte("ok")); err != nil {
		t.Fatal(err)
	}
	v, err := s.GetKV(ctx, "agent", "state")
	if err != nil || string(v) != "ok" {
		t.Fatalf("value=%q err=%v", v, err)
	}
}

func TestChunkTextDeterministic(t *testing.T) {
	text := "primeiro parágrafo. segundo parágrafo. terceiro parágrafo."
	a := ChunkText(text, 24, 4)
	b := ChunkText(text, 24, 4)
	if len(a) < 2 || len(a) != len(b) {
		t.Fatalf("unexpected chunks %#v %#v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non deterministic")
		}
	}
}

func TestFTSLifecycle(t *testing.T) {
	s := openTestStore(t, false)
	ctx := context.Background()
	m, err := s.AddMemory(ctx, MemoryInput{Content: "agente perdeu conexão com provider oauth"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Search(ctx, "provider", SearchOptions{})
	if err != nil || len(r) == 0 {
		t.Fatalf("results=%v err=%v", r, err)
	}
	if err := s.DeleteDocument(ctx, m.DocumentID); err != nil {
		t.Fatal(err)
	}
	r, err = s.Search(ctx, "provider", SearchOptions{})
	if err != nil || len(r) != 0 {
		t.Fatalf("after delete results=%v err=%v", r, err)
	}
}

func TestGraphCycleBounded(t *testing.T) {
	s := openTestStore(t, false)
	ctx := context.Background()
	a, _ := s.UpsertNode(ctx, Node{Kind: 1, Canonical: "A", Display: "A"})
	b, _ := s.UpsertNode(ctx, Node{Kind: 1, Canonical: "B", Display: "B"})
	c, _ := s.UpsertNode(ctx, Node{Kind: 1, Canonical: "C", Display: "C"})
	d, _ := s.UpsertNode(ctx, Node{Kind: 1, Canonical: "D", Display: "D"})
	for _, e := range []Edge{{a, b, 1, 1, 1}, {b, c, 1, 1, 1}, {c, d, 1, 1, 1}, {c, a, 1, 1, 1}} {
		if err := s.UpsertEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	nodes, err := s.Walk(ctx, a, 2, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 || len(nodes) > 3 {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
}

func TestQuantizeI8(t *testing.T) {
	v := make([]float32, VectorDimensions)
	v[0], v[1], v[2] = 1, -1, .5
	q, err := QuantizeI8(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != VectorDimensions || int8(q[0]) != 127 || int8(q[1]) != -127 {
		t.Fatalf("bad quantization: %v %v", int8(q[0]), int8(q[1]))
	}
	if math.Abs(float64(int8(q[2]))-64) > 1 {
		t.Fatalf("unexpected midpoint %d", int8(q[2]))
	}
}

func TestVectorInsertDelete(t *testing.T) {
	s := openTestStore(t, true)
	ctx := context.Background()
	m, err := s.AddMemory(ctx, MemoryInput{Content: "authentication provider failure"})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := s.processOnePending(ctx)
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	st, err := s.Stats(ctx)
	if err != nil || st.Vectors != 1 {
		t.Fatalf("stats=%+v err=%v", st, err)
	}
	if err := s.DeleteDocument(ctx, m.DocumentID); err != nil {
		t.Fatal(err)
	}
	st, err = s.Stats(ctx)
	if err != nil || st.Vectors != 0 || st.Chunks != 0 {
		t.Fatalf("after delete stats=%+v err=%v", st, err)
	}
}
