package micrographrag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) PutKV(ctx context.Context, namespace, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO kv(namespace,key,value,updated_at) VALUES(?,?,?,?)
ON CONFLICT(namespace,key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		namespace, key, value, unixNow())
	if err != nil {
		return fmt.Errorf("micrographrag: kv put: %w", err)
	}
	return nil
}

func (s *Store) GetKV(ctx context.Context, namespace, key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRowContext(ctx, "SELECT value FROM kv WHERE namespace=? AND key=?", namespace, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("micrographrag: kv get: %w", err)
	}
	return value, nil
}

func (s *Store) DeleteKV(ctx context.Context, namespace, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM kv WHERE namespace=? AND key=?", namespace, key)
	if err != nil {
		return fmt.Errorf("micrographrag: kv delete: %w", err)
	}
	return nil
}
