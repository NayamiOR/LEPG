package cache

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type SQLiteStore struct {
	db *bun.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	sqldb, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	if _, err := db.NewCreateTable().
		Model((*Reading)(nil)).
		IfNotExists().
		Exec(context.Background()); err != nil {
		sqldb.Close()
		return nil, err
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
