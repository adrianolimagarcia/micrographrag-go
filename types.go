package micrographrag

import "errors"

var (
	ErrNotFound       = errors.New("micrographrag: not found")
	ErrZeroVector     = errors.New("micrographrag: zero vector")
	ErrVectorDisabled = errors.New("micrographrag: vector search disabled")
	ErrFTSDisabled    = errors.New("micrographrag: fts disabled")
	ErrGraphDisabled  = errors.New("micrographrag: graph disabled")
)

const (
	EmbeddingPending = iota
	EmbeddingIndexed
	EmbeddingRetry
	EmbeddingUnavailable
	EmbeddingDisabled
)

type MemoryInput struct {
	Kind       int
	Source     string
	Title      string
	Content    string
	Metadata   []byte
	Importance float64
}

type MemoryResult struct {
	DocumentID int64
	ChunkIDs   []int64
}

type Node struct {
	ID        int64
	Kind      int
	Canonical string
	Display   string
	Metadata  []byte
}

type Edge struct {
	Src        int64
	Dst        int64
	Relation   int
	Weight     float64
	Confidence float64
}

type SearchOptions struct {
	Limit int

	EnableFTS    bool
	EnableVector bool
	EnableGraph  bool

	FTSLimit    int
	VectorLimit int
	GraphDepth  int

	FTSWeight    float64
	VectorWeight float64
}

type SearchResult struct {
	ChunkID    int64
	DocumentID int64
	Content    string
	Score      float64

	FTSRank        int
	VectorRank     int
	VectorDistance float64
	GraphDepth     int

	FromFTS    bool
	FromVector bool
	FromGraph  bool
}

type IntegrityReport struct {
	SQLiteOK          bool
	FTSOK             bool
	OrphanVectors     int64
	OrphanChunkNodes  int64
	PendingEmbeddings int64
}

type DBStats struct {
	Documents int64
	Chunks    int64
	Nodes     int64
	Edges     int64
	Vectors   int64
	Pending   int64
}
