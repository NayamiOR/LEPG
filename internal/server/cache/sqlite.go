package cache

import (
	"LEPG/internal/model"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"

	migrations "LEPG/internal/server/cache/migrations"
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
	sqldb.SetMaxOpenConns(1)

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

func (s *SQLiteStore) SaveReadings(ctx context.Context, sn string, readings []*model.Reading) error {
	if len(readings) == 0 {
		return nil
	}
	stored := make([]*StoredReading, len(readings))
	now := nowMs()
	for i, r := range readings {
		stored[i] = &StoredReading{
			Sn:         sn,
			UploadTime: now,
			Device:     r.Device,
			DeviceName: r.DeviceName,
			Point:      r.Point,
			PointName:  r.PointName,
			DataType:   r.DataType,
			Value:      r.Value,
			Quality:    r.Quality,
			Unit:       r.Unit,
			Timestamp:  r.Timestamp,
		}
	}
	_, err := s.db.NewInsert().Model(&stored).Exec(ctx)
	return err
}

func (s *SQLiteStore) QueryReadings(ctx context.Context, filter QueryFilter) ([]*StoredReading, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}

	q := s.db.NewSelect().Model((*StoredReading)(nil))

	if filter.Sn != "" {
		q = q.Where("sn = ?", filter.Sn)
	}
	if filter.Device != "" {
		q = q.Where("device = ?", filter.Device)
	}
	if filter.StartTime > 0 {
		q = q.Where("timestamp >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		q = q.Where("timestamp <= ?", filter.EndTime)
	}

	var readings []*StoredReading
	err := q.OrderExpr("timestamp DESC").Limit(limit).Scan(ctx)
	return readings, err
}
