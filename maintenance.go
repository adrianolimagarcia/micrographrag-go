package micrographrag

import (
	"context"
	"fmt"
)

func (s *Store) RebuildFTS(ctx context.Context) error {
	if !s.ftsEnabled {
		return ErrFTSDisabled
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO chunk_fts(chunk_fts) VALUES('rebuild')")
	if err != nil {
		return fmt.Errorf("micrographrag: rebuild FTS: %w", err)
	}
	return nil
}

func (s *Store) Optimize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return fmt.Errorf("micrographrag: optimize: %w", err)
	}
	return nil
}

func (s *Store) Vacuum(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("micrographrag: vacuum: %w", err)
	}
	return nil
}

func (s *Store) CheckIntegrity(ctx context.Context) (IntegrityReport, error) {
	var report IntegrityReport
	var result string
	if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return report, err
	}
	report.SQLiteOK = result == "ok"

	report.FTSOK = true
	if s.ftsEnabled {
		if _, err := s.db.ExecContext(ctx, "INSERT INTO chunk_fts(chunk_fts,rank) VALUES('integrity-check',1)"); err != nil {
			report.FTSOK = false
		}
	}
	if s.vectorEnabled {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunk_vec v LEFT JOIN chunks c ON c.id=v.rowid WHERE c.id IS NULL`).Scan(&report.OrphanVectors); err != nil {
			return report, err
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunk_nodes cn LEFT JOIN chunks c ON c.id=cn.chunk_id LEFT JOIN nodes n ON n.id=cn.node_id WHERE c.id IS NULL OR n.id IS NULL`).Scan(&report.OrphanChunkNodes); err != nil {
		return report, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks WHERE embedding_state IN (?,?)", EmbeddingPending, EmbeddingRetry).Scan(&report.PendingEmbeddings); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Store) Stats(ctx context.Context) (DBStats, error) {
	var st DBStats
	queries := []struct {
		q   string
		out *int64
	}{
		{"SELECT COUNT(*) FROM documents", &st.Documents},
		{"SELECT COUNT(*) FROM chunks", &st.Chunks},
		{"SELECT COUNT(*) FROM nodes", &st.Nodes},
		{"SELECT COUNT(*) FROM edges", &st.Edges},
		{"SELECT COUNT(*) FROM chunks WHERE embedding_state IN (0,2)", &st.Pending},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.q).Scan(item.out); err != nil {
			return st, err
		}
	}
	if s.vectorEnabled {
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunk_vec").Scan(&st.Vectors); err != nil {
			return st, err
		}
	}
	return st, nil
}
