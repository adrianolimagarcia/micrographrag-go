package micrographrag

import (
	"context"
	"fmt"
	"sort"
)

type rankedCandidate struct {
	SearchResult
}

func (s *Store) DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		Limit:        s.cfg.HybridLimit,
		EnableFTS:    s.ftsEnabled,
		EnableVector: s.vectorEnabled && s.embedder != nil,
		EnableGraph:  s.graphEnabled,
		FTSLimit:     s.cfg.FTSLimit,
		VectorLimit:  s.cfg.VectorLimit,
		GraphDepth:   s.cfg.GraphDepth,
		FTSWeight:    1,
		VectorWeight: 1,
	}
}

func normalizeSearchOptions(def, in SearchOptions) SearchOptions {
	if in.Limit <= 0 {
		in.Limit = def.Limit
	}
	if in.Limit > 12 {
		in.Limit = 12
	}
	if in.FTSLimit <= 0 {
		in.FTSLimit = def.FTSLimit
	}
	if in.FTSLimit > 64 {
		in.FTSLimit = 64
	}
	if in.VectorLimit <= 0 {
		in.VectorLimit = def.VectorLimit
	}
	if in.VectorLimit > 64 {
		in.VectorLimit = 64
	}
	if in.GraphDepth <= 0 {
		in.GraphDepth = def.GraphDepth
	}
	if in.GraphDepth > 3 {
		in.GraphDepth = 3
	}
	if in.FTSWeight == 0 {
		in.FTSWeight = 1
	}
	if in.VectorWeight == 0 {
		in.VectorWeight = 1
	}
	return in
}

func (s *Store) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}
	def := s.DefaultSearchOptions()
	if opts == (SearchOptions{}) {
		opts = def
	} else {
		opts = normalizeSearchOptions(def, opts)
	}

	candidates := make(map[int64]*rankedCandidate)
	if opts.EnableFTS && s.ftsEnabled {
		fts, err := s.searchFTS(ctx, query, opts.FTSLimit)
		if err != nil {
			return nil, err
		}
		for i, r := range fts {
			c := getCandidate(candidates, r)
			c.FromFTS = true
			c.FTSRank = i + 1
			c.Score += opts.FTSWeight / float64(60+i+1)
		}
	}

	if opts.EnableVector && s.vectorEnabled && s.embedder != nil {
		vec, err := s.searchVector(ctx, query, opts.VectorLimit)
		if err == nil {
			for i, r := range vec {
				c := getCandidate(candidates, r)
				c.FromVector = true
				c.VectorRank = i + 1
				c.VectorDistance = r.VectorDistance
				c.Score += opts.VectorWeight / float64(60+i+1)
			}
		}
	}

	ordered := flattenCandidates(candidates)
	seedLimit := opts.Limit
	if seedLimit > len(ordered) {
		seedLimit = len(ordered)
	}
	if opts.EnableGraph && s.graphEnabled && seedLimit > 0 {
		if err := s.expandGraph(ctx, candidates, ordered[:seedLimit], opts.GraphDepth); err != nil {
			return nil, err
		}
		ordered = flattenCandidates(candidates)
	}
	if len(ordered) > opts.Limit {
		ordered = ordered[:opts.Limit]
	}
	return ordered, nil
}

func getCandidate(m map[int64]*rankedCandidate, r SearchResult) *rankedCandidate {
	if c, ok := m[r.ChunkID]; ok {
		return c
	}
	c := &rankedCandidate{SearchResult: r}
	m[r.ChunkID] = c
	return c
}

func flattenCandidates(m map[int64]*rankedCandidate) []SearchResult {
	out := make([]SearchResult, 0, len(m))
	for _, c := range m {
		out = append(out, c.SearchResult)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ChunkID < out[j].ChunkID
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func (s *Store) searchFTS(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if !s.ftsEnabled {
		return nil, ErrFTSDisabled
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id,c.document_id,c.content,bm25(chunk_fts)
FROM chunk_fts
JOIN chunks c ON c.id=chunk_fts.rowid
WHERE chunk_fts MATCH ?
ORDER BY bm25(chunk_fts)
LIMIT ?`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: FTS query: %w", err)
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		var bm25 float64
		if err := rows.Scan(&r.ChunkID, &r.DocumentID, &r.Content, &bm25); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) searchVector(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if !s.vectorEnabled || s.embedder == nil {
		return nil, ErrVectorDisabled
	}
	v, err := s.embedder.Encode(ctx, query)
	if err != nil {
		return nil, err
	}
	q, err := QuantizeI8(v)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
WITH knn AS (
  SELECT rowid,distance
  FROM chunk_vec
  WHERE embedding MATCH vec_int8(?) AND k = ?
)
SELECT c.id,c.document_id,c.content,knn.distance
FROM knn JOIN chunks c ON c.id=knn.rowid
ORDER BY knn.distance`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: vector query: %w", err)
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ChunkID, &r.DocumentID, &r.Content, &r.VectorDistance); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) expandGraph(ctx context.Context, candidates map[int64]*rankedCandidate, seeds []SearchResult, depth int) error {
	if depth <= 0 {
		return nil
	}
	for _, seed := range seeds {
		rows, err := s.db.QueryContext(ctx, `
WITH RECURSIVE seed_nodes(id) AS (
  SELECT node_id FROM chunk_nodes WHERE chunk_id=? LIMIT 16
), walk(id,depth,path,score) AS (
  SELECT id,0,printf(',%lld,',id),1.0 FROM seed_nodes
  UNION ALL
  SELECT CASE WHEN e.src=w.id THEN e.dst ELSE e.src END,
         w.depth+1,
         w.path || CASE WHEN e.src=w.id THEN e.dst ELSE e.src END || ',',
         w.score * e.weight * e.confidence * 0.5
  FROM walk w
  JOIN edges e ON (e.src=w.id OR e.dst=w.id)
  WHERE w.depth < ?
    AND instr(w.path,printf(',%lld,',CASE WHEN e.src=w.id THEN e.dst ELSE e.src END))=0
), bounded AS (
  SELECT id,MIN(depth) AS depth,MAX(score) AS score
  FROM walk GROUP BY id LIMIT ?
)
SELECT c.id,c.document_id,c.content,b.depth,b.score,cn.weight
FROM bounded b
JOIN chunk_nodes cn ON cn.node_id=b.id
JOIN chunks c ON c.id=cn.chunk_id
WHERE c.id<>?
LIMIT 32`, seed.ChunkID, depth, s.cfg.GraphMaxNodes, seed.ChunkID)
		if err != nil {
			return fmt.Errorf("micrographrag: graph expansion: %w", err)
		}
		for rows.Next() {
			var r SearchResult
			var graphScore, chunkWeight float64
			if err := rows.Scan(&r.ChunkID, &r.DocumentID, &r.Content, &r.GraphDepth, &graphScore, &chunkWeight); err != nil {
				rows.Close()
				return err
			}
			c := getCandidate(candidates, r)
			c.FromGraph = true
			if c.GraphDepth == 0 || r.GraphDepth < c.GraphDepth {
				c.GraphDepth = r.GraphDepth
			}
			boost := seed.Score * graphScore * chunkWeight
			if boost > seed.Score*0.5 {
				boost = seed.Score * 0.5
			}
			c.Score += boost
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}
