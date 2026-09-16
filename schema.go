package micrographrag

import (
	"context"
	"fmt"
)

func (s *Store) createSchema(ctx context.Context) error {
	base := `
CREATE TABLE IF NOT EXISTS schema_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
) WITHOUT ROWID;
INSERT OR IGNORE INTO schema_meta(key,value) VALUES('schema_version','1');

CREATE TABLE IF NOT EXISTS kv (
  namespace TEXT NOT NULL,
  key TEXT NOT NULL,
  value BLOB NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY(namespace,key)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY,
  kind INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  metadata BLOB,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS chunks (
  id INTEGER PRIMARY KEY,
  document_id INTEGER,
  seq INTEGER NOT NULL DEFAULT 0,
  content TEXT NOT NULL,
  content_hash BLOB,
  importance REAL NOT NULL DEFAULT 1.0,
  embedding_state INTEGER NOT NULL DEFAULT 0,
  embedding_attempts INTEGER NOT NULL DEFAULT 0,
  embedding_retry_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY(document_id) REFERENCES documents(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chunks_document ON chunks(document_id,seq);
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_pending ON chunks(embedding_state,embedding_retry_at);

CREATE TABLE IF NOT EXISTS nodes (
  id INTEGER PRIMARY KEY,
  kind INTEGER NOT NULL,
  canonical TEXT NOT NULL,
  display TEXT NOT NULL DEFAULT '',
  metadata BLOB,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(kind,canonical)
);

CREATE TABLE IF NOT EXISTS edges (
  src INTEGER NOT NULL,
  dst INTEGER NOT NULL,
  relation INTEGER NOT NULL,
  weight REAL NOT NULL DEFAULT 1.0,
  confidence REAL NOT NULL DEFAULT 1.0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY(src,dst,relation),
  FOREIGN KEY(src) REFERENCES nodes(id) ON DELETE CASCADE,
  FOREIGN KEY(dst) REFERENCES nodes(id) ON DELETE CASCADE
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst,relation);

CREATE TABLE IF NOT EXISTS chunk_nodes (
  chunk_id INTEGER NOT NULL,
  node_id INTEGER NOT NULL,
  weight REAL NOT NULL DEFAULT 1.0,
  PRIMARY KEY(chunk_id,node_id),
  FOREIGN KEY(chunk_id) REFERENCES chunks(id) ON DELETE CASCADE,
  FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_chunk_nodes_node ON chunk_nodes(node_id,chunk_id);
`
	if _, err := s.db.ExecContext(ctx, base); err != nil {
		return fmt.Errorf("micrographrag: create base schema: %w", err)
	}

	if s.ftsEnabled {
		fts := `
CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
  content,
  content='chunks',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
  INSERT INTO chunk_fts(rowid,content) VALUES(new.id,new.content);
END;
CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
  INSERT INTO chunk_fts(chunk_fts,rowid,content) VALUES('delete',old.id,old.content);
END;
CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE OF content ON chunks BEGIN
  INSERT INTO chunk_fts(chunk_fts,rowid,content) VALUES('delete',old.id,old.content);
  INSERT INTO chunk_fts(rowid,content) VALUES(new.id,new.content);
END;
`
		if _, err := s.db.ExecContext(ctx, fts); err != nil {
			return fmt.Errorf("micrographrag: create FTS schema: %w", err)
		}
	}

	if s.vectorEnabled {
		vec := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS chunk_vec USING vec0(
  embedding int8[%d] distance_metric=cosine
);`, VectorDimensions)
		if _, err := s.db.ExecContext(ctx, vec); err != nil {
			return fmt.Errorf("micrographrag: create vector schema: %w", err)
		}
	}
	return nil
}
