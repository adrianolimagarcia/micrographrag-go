# micrographrag-go

A low-memory embedded memory/database layer for Go agents.

It combines **SQLite + FTS5 + graph tables + sqlite-vec + local static embeddings** in a single SQLite database. The target is an agent that already runs around 12 MB RSS and should remain below roughly 30 MB total under normal embedded workloads.

## Architecture

```text
                     Go Agent
                        |
                  +-----+------+
                  | MicroStore |
                  +-----+------+
                        |
           +------------+-------------+
           |            |             |
          KV           FTS5          Graph
                                      nodes/edges
           |            |             |
           |       Local Embedder      |
           |       Model2Vec 64D       |
           |            |             |
           |       INT8 quantize       |
           |            |             |
           |        sqlite-vec         |
           |            |             |
           +--------- RRF -------------+
                        |
                 graph expansion
                    1-2 hops
                        |
                  Agent Context
```

## Design goals

- one `agent.db`, no database server;
- SQLite is also the KV/document/state store;
- FTS5 provides lexical/BM25 retrieval;
- nodes + edges + recursive CTE provide bounded graph traversal;
- `chunk_nodes` connects graph entities to textual memories;
- sqlite-vec stores compact `int8[64]` vectors;
- Model2Vec/Potion generates 64D embeddings locally;
- embeddings are generated lazily after the memory transaction commits;
- RRF merges lexical and semantic ranks without mixing incompatible score scales;
- every expensive operation has small hard limits;
- FTS/database keep working if embeddings are unavailable.

## Requirements

- Go 1.25+
- C compiler (CGO)
- build with the `sqlite_fts5` tag

The sqlite-vec CGO binding compiles the extension into the application; there is no runtime extension file or vector server.

## Quick start

```bash
go mod download
make test
make build
```

Minimal database-only usage:

```go
cfg := micrographrag.DefaultConfig("agent.db")
cfg.EnableVector = false

store, err := micrographrag.Open(ctx, cfg, nil)
if err != nil { panic(err) }
defer store.Close()

_ = store.PutKV(ctx, "agent", "state", []byte("ready"))
```

With local embeddings:

```go
embedder, err := micrographrag.NewPotionEmbedder(ctx)
if err != nil { panic(err) }

store, err := micrographrag.Open(
    ctx,
    micrographrag.DefaultConfig("agent.db"),
    embedder,
)
```

## Fully offline model deployment

`go-potion` normally downloads `potion-base-2M` on the first run. For an embedded/offline deployment, preload these files:

```text
$GO_POTION_HOME/BASE2M/model.safetensors
$GO_POTION_HOME/BASE2M/tokenizer.json
```

When both files exist, model loading performs no download. A custom compatible 64D Model2Vec PT/EN model can be placed in the same location. See `scripts/build_embedding_model/`.

## Memory insertion

```go
m, err := store.AddMemory(ctx, micrographrag.MemoryInput{
    Source:  "tool",
    Title:   "OAuth failure",
    Content: "O provider recusou a autenticação OAuth.",
})
```

The transaction writes the document/chunks and FTS index first. Vector embedding happens afterward through a single lazy worker, so a slow embedder does not keep the write transaction open.

## Graph

```go
provider, _ := store.UpsertNode(ctx, micrographrag.Node{
    Kind: 1, Canonical: "provider", Display: "Provider",
})

oauth, _ := store.UpsertNode(ctx, micrographrag.Node{
    Kind: 2, Canonical: "oauth", Display: "OAuth",
})

_ = store.UpsertEdge(ctx, micrographrag.Edge{
    Src: provider, Dst: oauth, Relation: 1, Weight: 1, Confidence: 1,
})

_ = store.LinkChunkNode(ctx, m.ChunkIDs[0], oauth, 1)
```

Traversal is bounded to depth <= 3 and <= 128 nodes; defaults are depth 2 and 64 nodes.

## Hybrid search

```go
results, err := store.Search(
    ctx,
    "problema de autenticação do provider",
    micrographrag.SearchOptions{},
)
```

Pipeline:

```text
FTS5 top 16 ---------+
                     +--> RRF(k=60) --> top 8 --> graph expansion --> final
Vector top 16 -------+
```

Vector failure is fail-soft: the same query can still return FTS + graph results.

## Important resource defaults

| Resource | Default |
|---|---:|
| SQLite page cache | 1536 KiB |
| Open SQLite connections | 1 |
| FTS candidates | 16 (hard max 64) |
| Vector candidates | 16 (hard max 64) |
| Final results | 8 (hard max 12) |
| Graph depth | 2 (hard max 3) |
| Graph visited nodes | 64 (hard max 128) |
| Vector dimension | 64 |
| Vector representation | INT8 |
| Embedding workers | 1 |
| `temp_store` | FILE |
| `mmap_size` for SQLite | 0 |

The Potion model itself uses mmap where supported. RSS must be measured on the actual target device; mmap-backed pages are not the same thing as Go heap.

## FTS consistency

FTS5 uses an external-content table backed by `chunks` and triggers keep it in sync. `RebuildFTS` and `CheckIntegrity` are included for recovery/diagnostics.

## Model changes

Do not mix vectors from different embedding models. v1 fixes the vector space to 64 dimensions. When replacing the local model, clear/rebuild `chunk_vec` and mark existing chunks pending before using vector retrieval again.

## Build targets

```bash
CGO_ENABLED=1 go test -tags sqlite_fts5 ./...
CGO_ENABLED=1 go build -tags sqlite_fts5 ./...
```

Cross-compilation requires a C toolchain for the target architecture.

## Status

This is an initial embedded-focused implementation. sqlite-vec is pre-v1, so its usage is intentionally isolated behind this package. Benchmark RSS and latency on your target hardware before setting production limits.
