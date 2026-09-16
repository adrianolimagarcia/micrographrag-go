package micrographrag

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) AddMemory(ctx context.Context, in MemoryInput) (MemoryResult, error) {
	chunks := ChunkText(in.Content, s.cfg.MaxChunkRunes, s.cfg.ChunkOverlap)
	if len(chunks) == 0 {
		return MemoryResult{}, fmt.Errorf("micrographrag: memory content is empty")
	}
	importance := in.Importance
	if importance <= 0 {
		importance = 1
	}
	state := EmbeddingDisabled
	if s.vectorEnabled && s.embedder != nil {
		state = EmbeddingPending
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MemoryResult{}, fmt.Errorf("micrographrag: begin memory transaction: %w", err)
	}
	defer tx.Rollback()

	now := unixNow()
	res, err := tx.ExecContext(ctx, `
INSERT INTO documents(kind,source,title,metadata,created_at,updated_at)
VALUES(?,?,?,?,?,?)`, in.Kind, in.Source, in.Title, in.Metadata, now, now)
	if err != nil {
		return MemoryResult{}, fmt.Errorf("micrographrag: insert document: %w", err)
	}
	docID, err := res.LastInsertId()
	if err != nil {
		return MemoryResult{}, fmt.Errorf("micrographrag: document id: %w", err)
	}

	ids := make([]int64, 0, len(chunks))
	for i, content := range chunks {
		h := sha256.Sum256([]byte(content))
		res, err := tx.ExecContext(ctx, `
INSERT INTO chunks(document_id,seq,content,content_hash,importance,embedding_state,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?)`, docID, i, content, h[:], importance, state, now, now)
		if err != nil {
			return MemoryResult{}, fmt.Errorf("micrographrag: insert chunk %d: %w", i, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return MemoryResult{}, fmt.Errorf("micrographrag: chunk id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return MemoryResult{}, fmt.Errorf("micrographrag: commit memory: %w", err)
	}
	if state == EmbeddingPending {
		s.signalEmbedder()
	}
	return MemoryResult{DocumentID: docID, ChunkIDs: ids}, nil
}

func (s *Store) DeleteDocument(ctx context.Context, documentID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if s.vectorEnabled {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_vec WHERE rowid IN (SELECT id FROM chunks WHERE document_id=?)`, documentID); err != nil {
			return fmt.Errorf("micrographrag: delete document vectors: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM documents WHERE id=?", documentID); err != nil {
		return fmt.Errorf("micrographrag: delete document: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("micrographrag: commit document delete: %w", err)
	}
	return nil
}

func (s *Store) startEmbeddingWorker() {
	s.workerWG.Add(1)
	go func() {
		defer s.workerWG.Done()
		ticker := time.NewTicker(s.cfg.EmbeddingPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopEmbed:
				return
			case <-s.wakeEmbed:
				s.drainPendingEmbeddings()
			case <-ticker.C:
				s.drainPendingEmbeddings()
			}
		}
	}()
}

func (s *Store) drainPendingEmbeddings() {
	for {
		processed, err := s.processOnePending(context.Background())
		if err != nil || !processed {
			return
		}
	}
}

func (s *Store) processOnePending(ctx context.Context) (bool, error) {
	var id int64
	var content string
	var attempts int
	err := s.db.QueryRowContext(ctx, `
SELECT id,content,embedding_attempts
FROM chunks
WHERE embedding_state IN (?,?) AND embedding_retry_at <= ?
ORDER BY id
LIMIT 1`, EmbeddingPending, EmbeddingRetry, unixNow()).Scan(&id, &content, &attempts)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	v, err := s.embedder.Encode(ctx, content)
	if err == nil {
		var q []byte
		q, err = QuantizeI8(v)
		if err == nil {
			tx, txErr := s.db.BeginTx(ctx, nil)
			if txErr != nil {
				return false, txErr
			}
			defer tx.Rollback()
			if _, txErr = tx.ExecContext(ctx, "INSERT OR REPLACE INTO chunk_vec(rowid,embedding) VALUES(?,vec_int8(?))", id, q); txErr == nil {
				_, txErr = tx.ExecContext(ctx, "UPDATE chunks SET embedding_state=?,updated_at=? WHERE id=?", EmbeddingIndexed, unixNow(), id)
			}
			if txErr != nil {
				return false, txErr
			}
			if txErr = tx.Commit(); txErr != nil {
				return false, txErr
			}
			return true, nil
		}
	}

	attempts++
	state := EmbeddingRetry
	retryAt := unixNow() + retryDelaySeconds(attempts)
	if attempts >= s.cfg.EmbeddingMaxAttempts {
		state = EmbeddingUnavailable
		retryAt = 0
	}
	_, updateErr := s.db.ExecContext(ctx, `
UPDATE chunks SET embedding_state=?,embedding_attempts=?,embedding_retry_at=?,updated_at=? WHERE id=?`,
		state, attempts, retryAt, unixNow(), id)
	if updateErr != nil {
		return false, updateErr
	}
	return true, nil
}

func retryDelaySeconds(attempt int) int64 {
	switch attempt {
	case 1:
		return 60
	case 2:
		return 300
	case 3:
		return 1800
	default:
		return 7200
	}
}
