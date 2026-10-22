package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Check struct {
	ID            int64
	Owner         string
	Repo          string
	Description   string
	License       string
	Stars         int
	Score         int
	Grade         string
	CheckedAt     time.Time
	ExternalLinks bool
}

type Store interface {
	Save(ctx context.Context, check Check) error
	Recent(ctx context.Context, limit int) ([]Check, error)
	Close() error
}

type SQLite struct {
	db *sql.DB
}

var _ Store = (*SQLite)(nil)

func Open(path string) (*SQLite, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path is empty")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &SQLite{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLite) migrate() error {
	if _, err := s.db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			license TEXT NOT NULL DEFAULT '',
			stars INTEGER NOT NULL DEFAULT 0,
			score INTEGER NOT NULL,
			grade TEXT NOT NULL,
			checked_at INTEGER NOT NULL,
			external_links INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS checks_repository
			ON checks (lower(owner), lower(repo), checked_at DESC);
	`)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func (s *SQLite) Save(ctx context.Context, check Check) error {
	if strings.TrimSpace(check.Owner) == "" || strings.TrimSpace(check.Repo) == "" {
		return fmt.Errorf("check owner and repo are required")
	}
	if check.CheckedAt.IsZero() {
		check.CheckedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO checks (owner, repo, description, license, stars, score, grade, checked_at, external_links)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		check.Owner,
		check.Repo,
		check.Description,
		check.License,
		check.Stars,
		check.Score,
		check.Grade,
		check.CheckedAt.UTC().UnixMilli(),
		boolToInt(check.ExternalLinks),
	)
	if err != nil {
		return fmt.Errorf("save check: %w", err)
	}
	return nil
}

func (s *SQLite) Recent(ctx context.Context, limit int) ([]Check, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner, repo, description, license, stars, score, grade, checked_at, external_links
		FROM checks
		WHERE id IN (
			SELECT MAX(id) FROM checks GROUP BY lower(owner), lower(repo)
		)
		ORDER BY checked_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent checks: %w", err)
	}
	defer rows.Close()

	var checks []Check
	for rows.Next() {
		var check Check
		var checkedAt int64
		var external int
		if err := rows.Scan(
			&check.ID,
			&check.Owner,
			&check.Repo,
			&check.Description,
			&check.License,
			&check.Stars,
			&check.Score,
			&check.Grade,
			&checkedAt,
			&external,
		); err != nil {
			return nil, fmt.Errorf("scan check: %w", err)
		}
		check.CheckedAt = time.UnixMilli(checkedAt).UTC()
		check.ExternalLinks = external != 0
		checks = append(checks, check)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checks: %w", err)
	}
	return checks, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
