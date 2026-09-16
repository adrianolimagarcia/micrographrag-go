package micrographrag

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var registerVec sync.Once

type Store struct {
	db       *sql.DB
	cfg      Config
	embedder Embedder

	vectorEnabled bool
	ftsEnabled    bool
	graphEnabled  bool

	wakeEmbed chan struct{}
	stopEmbed chan struct{}
	workerWG  sync.WaitGroup
	closeOnce sync.Once
}

func Open(ctx context.Context, cfg Config, embedder Embedder) (*Store, error) {
	cfg.normalize()
	registerVec.Do(sqlite_vec.Auto)

	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("micrographrag: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	s := &Store{
		db:           db,
		cfg:          cfg,
		embedder:     embedder,
		ftsEnabled:   cfg.EnableFTS,
		graphEnabled: cfg.EnableGraph,
		wakeEmbed:    make(chan struct{}, 1),
		stopEmbed:    make(chan struct{}),
	}

	if err := s.applyPragmas(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.detectFeatures(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.createSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if s.vectorEnabled && s.embedder != nil {
		if s.embedder.Dimensions() != VectorDimensions {
			db.Close()
			return nil, fmt.Errorf("micrographrag: embedder dimensions=%d, want=%d", s.embedder.Dimensions(), VectorDimensions)
		}
		if s.cfg.EnableEmbeddingWorker {
			s.startEmbeddingWorker()
		}
	}
	return s, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.stopEmbed)
		s.workerWG.Wait()
		if s.embedder != nil {
			if e := s.embedder.Close(); e != nil {
				err = e
			}
		}
		if e := s.db.Close(); err == nil {
			err = e
		}
	})
	return err
}

func (s *Store) applyPragmas(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA foreign_keys=ON",
		fmt.Sprintf("PRAGMA busy_timeout=%d", s.cfg.BusyTimeout.Milliseconds()),
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=FILE",
		fmt.Sprintf("PRAGMA cache_size=-%d", s.cfg.SQLiteCacheKB),
		"PRAGMA cache_spill=ON",
		"PRAGMA mmap_size=0",
	}
	for _, q := range pragmas {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("micrographrag: %s: %w", q, err)
		}
	}
	if s.cfg.EnableWAL {
		var mode string
		if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err == nil && mode == "wal" {
			_, _ = s.db.ExecContext(ctx, "PRAGMA wal_autocheckpoint=64")
			_, _ = s.db.ExecContext(ctx, "PRAGMA journal_size_limit=1048576")
		} else {
			_, _ = s.db.ExecContext(ctx, "PRAGMA journal_mode=DELETE")
		}
	}
	return nil
}

func (s *Store) detectFeatures(ctx context.Context) error {
	if s.cfg.EnableFTS {
		var enabled int
		if err := s.db.QueryRowContext(ctx, "SELECT sqlite_compileoption_used('ENABLE_FTS5')").Scan(&enabled); err != nil {
			return fmt.Errorf("micrographrag: detect FTS5: %w", err)
		}
		if enabled == 0 {
			return fmt.Errorf("micrographrag: FTS5 not compiled; build with -tags sqlite_fts5")
		}
	}

	s.vectorEnabled = false
	if s.cfg.EnableVector && s.embedder != nil {
		var version string
		if err := s.db.QueryRowContext(ctx, "SELECT vec_version()").Scan(&version); err == nil && version != "" {
			s.vectorEnabled = true
		}
	}
	return nil
}

func (s *Store) signalEmbedder() {
	select {
	case s.wakeEmbed <- struct{}{}:
	default:
	}
}

func unixNow() int64 { return time.Now().Unix() }
