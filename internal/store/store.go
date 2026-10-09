// Package store is the SQLite persistence layer. It contains SQL only; rules live in service.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrDuplicate is returned when a write violates a uniqueness constraint.
var ErrDuplicate = errors.New("store: duplicate")

// Store owns two pools on one database file: a single writer connection that
// serializes all writes, and a reader pool (WAL lets readers run during a write).
type Store struct {
	writer *sql.DB
	reader *sql.DB
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	writer, err := openPool(ctx, path, false)
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	if err := migrate(ctx, writer); err != nil {
		_ = writer.Close()
		return nil, err
	}
	reader, err := openPool(ctx, path, true)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	return &Store{writer: writer, reader: reader}, nil
}

// Close closes both pools.
func (s *Store) Close() error {
	return errors.Join(s.reader.Close(), s.writer.Close())
}

// openPool applies pragmas per connection through the DSN, since they do not
// persist across pooled connections otherwise.
func openPool(ctx context.Context, path string, readOnly bool) (*sql.DB, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	if readOnly {
		q.Add("_pragma", "query_only(1)")
	}
	dsn := (&url.URL{Scheme: "file", Opaque: filepath.ToSlash(path), RawQuery: q.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations fs: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Write runs fn in a transaction on the writer connection; it commits if fn
// returns nil and rolls back otherwise.
func (s *Store) Write(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(&Tx{tx: tx}); err != nil {
		return errors.Join(err, rollback(tx))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}
