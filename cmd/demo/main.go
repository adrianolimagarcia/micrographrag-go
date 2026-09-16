package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	micro "github.com/adrianolimagarcia/micrographrag-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := "demo.db"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	embedder, err := micro.NewPotionEmbedder(ctx)
	if err != nil {
		log.Printf("embedder unavailable; continuing with FTS + graph: %v", err)
		embedder = nil
	}

	cfg := micro.DefaultConfig(path)
	store, err := micro.Open(ctx, cfg, embedder)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	mem, err := store.AddMemory(ctx, micro.MemoryInput{
		Source:  "demo",
		Title:   "Provider failure",
		Content: "O agente perdeu conexão com o provider OAuth e precisa tentar autenticar novamente.",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("document=%d chunks=%v\n", mem.DocumentID, mem.ChunkIDs)

	results, err := store.Search(ctx, "problema de autenticação provider", micro.SearchOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Printf("score=%.5f chunk=%d fts=%v vector=%v graph=%v: %s\n",
			r.Score, r.ChunkID, r.FromFTS, r.FromVector, r.FromGraph, r.Content)
	}
}
