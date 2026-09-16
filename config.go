package micrographrag

import "time"

const VectorDimensions = 64

type Config struct {
	DBPath string

	SQLiteCacheKB int
	BusyTimeout   time.Duration
	EnableWAL     bool

	EnableFTS    bool
	EnableVector bool
	EnableGraph  bool

	FTSLimit    int
	VectorLimit int
	HybridLimit int

	GraphDepth    int
	GraphMaxNodes int

	MaxChunkRunes int
	ChunkOverlap  int

	EmbeddingPollInterval time.Duration
	EmbeddingMaxAttempts  int
	EnableEmbeddingWorker bool
}

func DefaultConfig(path string) Config {
	return Config{
		DBPath:                path,
		SQLiteCacheKB:         1536,
		BusyTimeout:           5 * time.Second,
		EnableWAL:             true,
		EnableFTS:             true,
		EnableVector:          true,
		EnableGraph:           true,
		FTSLimit:              16,
		VectorLimit:           16,
		HybridLimit:           8,
		GraphDepth:            2,
		GraphMaxNodes:         64,
		MaxChunkRunes:         1200,
		ChunkOverlap:          120,
		EmbeddingPollInterval: 5 * time.Second,
		EmbeddingMaxAttempts:  4,
		EnableEmbeddingWorker: true,
	}
}

func (c *Config) normalize() {
	if c.DBPath == "" { c.DBPath = "agent.db" }
	if c.SQLiteCacheKB <= 0 { c.SQLiteCacheKB = 1536 }
	if c.BusyTimeout <= 0 { c.BusyTimeout = 5 * time.Second }
	if c.FTSLimit <= 0 { c.FTSLimit = 16 }
	if c.FTSLimit > 64 { c.FTSLimit = 64 }
	if c.VectorLimit <= 0 { c.VectorLimit = 16 }
	if c.VectorLimit > 64 { c.VectorLimit = 64 }
	if c.HybridLimit <= 0 { c.HybridLimit = 8 }
	if c.HybridLimit > 12 { c.HybridLimit = 12 }
	if c.GraphDepth <= 0 { c.GraphDepth = 2 }
	if c.GraphDepth > 3 { c.GraphDepth = 3 }
	if c.GraphMaxNodes <= 0 { c.GraphMaxNodes = 64 }
	if c.GraphMaxNodes > 128 { c.GraphMaxNodes = 128 }
	if c.MaxChunkRunes <= 0 { c.MaxChunkRunes = 1200 }
	if c.ChunkOverlap < 0 { c.ChunkOverlap = 0 }
	if c.ChunkOverlap >= c.MaxChunkRunes { c.ChunkOverlap = c.MaxChunkRunes / 10 }
	if c.EmbeddingPollInterval <= 0 { c.EmbeddingPollInterval = 5 * time.Second }
	if c.EmbeddingMaxAttempts <= 0 { c.EmbeddingMaxAttempts = 4 }
}
