package cache

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"

	migrations "LEPG/internal/client/cache/migrations"
)

type SQLiteStore struct {
	db *bun.DB
}

func NewSQLiteStore(ctx context.Context, dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	if err := sqldb.Ping(); err != nil {
		sqldb.Close()
		return nil, err
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	migrator := migrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("init migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) SaveReadings(ctx context.Context, readings []*Reading) error {
	if len(readings) == 0 {
		return nil
	}
	_, err := s.db.NewInsert().Model(&readings).Exec(ctx)
	return err
}

func (s *SQLiteStore) LoadReadings(ctx context.Context, limit int) ([]*Reading, error) {
	var readings []*Reading
	err := s.db.NewSelect().
		Model(&readings).
		OrderExpr("id ASC").
		Limit(limit).
		Scan(ctx)
	return readings, err
}

func (s *SQLiteStore) LoadPendingReadings(ctx context.Context, limit int) ([]*Reading, error) {
	var readings []*Reading
	err := s.db.NewSelect().
		Model(&readings).
		Where("status IN (?, ?)", 0, 3).
		OrderExpr("id ASC").
		Limit(limit).
		Scan(ctx)
	return readings, err
}

func (s *SQLiteStore) UpdateReadingsStatus(ctx context.Context, ids []int64, status int) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.NewUpdate().
		Model((*Reading)(nil)).
		Set("status = ?", status).
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	return err
}

func (s *SQLiteStore) DeleteReadings(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.NewDelete().
		Model((*Reading)(nil)).
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	return err
}
