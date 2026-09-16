package micrographrag

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func normalizeCanonical(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (s *Store) UpsertNode(ctx context.Context, n Node) (int64, error) {
	if !s.graphEnabled {
		return 0, ErrGraphDisabled
	}
	canonical := normalizeCanonical(n.Canonical)
	if canonical == "" {
		return 0, fmt.Errorf("micrographrag: empty node canonical name")
	}
	now := unixNow()
	if n.ID != 0 {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO nodes(id,kind,canonical,display,metadata,created_at,updated_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET kind=excluded.kind,canonical=excluded.canonical,display=excluded.display,metadata=excluded.metadata,updated_at=excluded.updated_at`,
			n.ID, n.Kind, canonical, n.Display, n.Metadata, now, now)
		if err != nil {
			return 0, fmt.Errorf("micrographrag: upsert node: %w", err)
		}
		return n.ID, nil
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO nodes(kind,canonical,display,metadata,created_at,updated_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(kind,canonical) DO UPDATE SET display=excluded.display,metadata=excluded.metadata,updated_at=excluded.updated_at`,
		n.Kind, canonical, n.Display, n.Metadata, now, now)
	if err != nil {
		return 0, fmt.Errorf("micrographrag: upsert node: %w", err)
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM nodes WHERE kind=? AND canonical=?", n.Kind, canonical).Scan(&id); err != nil {
		return 0, fmt.Errorf("micrographrag: lookup upserted node: %w", err)
	}
	return id, nil
}

func (s *Store) UpsertEdge(ctx context.Context, e Edge) error {
	if !s.graphEnabled {
		return ErrGraphDisabled
	}
	if e.Weight == 0 {
		e.Weight = 1
	}
	if e.Confidence == 0 {
		e.Confidence = 1
	}
	now := unixNow()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO edges(src,dst,relation,weight,confidence,created_at,updated_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(src,dst,relation) DO UPDATE SET weight=excluded.weight,confidence=excluded.confidence,updated_at=excluded.updated_at`,
		e.Src, e.Dst, e.Relation, e.Weight, e.Confidence, now, now)
	if err != nil {
		return fmt.Errorf("micrographrag: upsert edge: %w", err)
	}
	return nil
}

func (s *Store) LinkChunkNode(ctx context.Context, chunkID, nodeID int64, weight float64) error {
	if !s.graphEnabled {
		return ErrGraphDisabled
	}
	if weight == 0 {
		weight = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chunk_nodes(chunk_id,node_id,weight) VALUES(?,?,?)
ON CONFLICT(chunk_id,node_id) DO UPDATE SET weight=excluded.weight`, chunkID, nodeID, weight)
	if err != nil {
		return fmt.Errorf("micrographrag: link chunk node: %w", err)
	}
	return nil
}

func (s *Store) Neighbors(ctx context.Context, nodeID int64, relation *int, limit int) ([]Node, error) {
	if !s.graphEnabled {
		return nil, ErrGraphDisabled
	}
	if limit <= 0 || limit > s.cfg.GraphMaxNodes {
		limit = s.cfg.GraphMaxNodes
	}
	query := `
SELECT n.id,n.kind,n.canonical,n.display,n.metadata
FROM nodes n
JOIN (
  SELECT dst AS id,weight FROM edges WHERE src=? AND (? IS NULL OR relation=?)
  UNION ALL
  SELECT src AS id,weight FROM edges WHERE dst=? AND (? IS NULL OR relation=?)
) x ON x.id=n.id
GROUP BY n.id
ORDER BY MAX(x.weight) DESC,n.id
LIMIT ?`
	var rel any
	if relation != nil {
		rel = *relation
	}
	rows, err := s.db.QueryContext(ctx, query, nodeID, rel, rel, nodeID, rel, rel, limit)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: neighbors: %w", err)
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Kind, &n.Canonical, &n.Display, &n.Metadata); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Walk(ctx context.Context, startID int64, depth, limit int) ([]Node, error) {
	if !s.graphEnabled {
		return nil, ErrGraphDisabled
	}
	if depth <= 0 {
		depth = s.cfg.GraphDepth
	}
	if depth > 3 {
		depth = 3
	}
	if limit <= 0 || limit > s.cfg.GraphMaxNodes {
		limit = s.cfg.GraphMaxNodes
	}
	rows, err := s.db.QueryContext(ctx, `
WITH RECURSIVE walk(id,depth,path) AS (
  SELECT ?,0,printf(',%lld,',?)
  UNION ALL
  SELECT CASE WHEN e.src=w.id THEN e.dst ELSE e.src END,
         w.depth+1,
         w.path || CASE WHEN e.src=w.id THEN e.dst ELSE e.src END || ','
  FROM walk w
  JOIN edges e ON (e.src=w.id OR e.dst=w.id)
  WHERE w.depth < ?
    AND instr(w.path,printf(',%lld,',CASE WHEN e.src=w.id THEN e.dst ELSE e.src END))=0
)
SELECT n.id,n.kind,n.canonical,n.display,n.metadata,MIN(w.depth)
FROM walk w JOIN nodes n ON n.id=w.id
WHERE w.depth>0
GROUP BY n.id
ORDER BY MIN(w.depth),n.id
LIMIT ?`, startID, startID, depth, limit)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: graph walk: %w", err)
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		var d int
		if err := rows.Scan(&n.ID, &n.Kind, &n.Canonical, &n.Display, &n.Metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func scanNode(row *sql.Row) (Node, error) {
	var n Node
	err := row.Scan(&n.ID, &n.Kind, &n.Canonical, &n.Display, &n.Metadata)
	if err != nil {
		return Node{}, err
	}
	return n, nil
}
