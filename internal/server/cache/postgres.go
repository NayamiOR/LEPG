package cache

import (
	"LEPG/internal/model"
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/migrate"

	migrations "LEPG/internal/server/cache/migrations"
)

type PostgresStore struct {
	db *bun.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	connector := pgdriver.NewConnector(pgdriver.WithDSN(dsn))
	sqldb := sql.OpenDB(connector)

	if err := sqldb.Ping(); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	db := bun.NewDB(sqldb, pgdialect.New())

	migrator := migrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("init migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) QueryDevices(ctx context.Context, filter DeviceQueryFilter) ([]*StoredDevice, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}

	var devices []*StoredDevice
	q := s.db.NewSelect().Model(&devices)

	if filter.Sn != "" {
		q = q.Where("sn = ?", filter.Sn)
	}

	err := q.OrderExpr("last_seen DESC").Limit(limit).Scan(ctx)
	return devices, err
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) SaveReadings(ctx context.Context, sn string, readings []*model.Reading) error {
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
	if err != nil {
		return err
	}

	// Upsert device registry: register each unique device seen in this batch
	type deviceKey struct{ deviceHash string }
	seen := make(map[deviceKey]string) // deviceKey -> deviceName
	for _, r := range readings {
		seen[deviceKey{r.Device}] = r.DeviceName
	}
	for dk, deviceName := range seen {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO devices (sn, device_hash, device_name, first_seen, last_seen, status)
			 VALUES (?, ?, ?, ?, ?, 'unknown')
			 ON CONFLICT(sn, device_hash) DO UPDATE SET
			 last_seen = excluded.last_seen,
			 device_name = excluded.device_name`,
			sn, dk.deviceHash, deviceName, now, now)
		if err != nil {
			return fmt.Errorf("upsert device %s: %w", dk.deviceHash, err)
		}
	}
	return nil
}

func (s *PostgresStore) QueryReadings(ctx context.Context, filter QueryFilter) ([]*StoredReading, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}

	var readings []*StoredReading
	q := s.db.NewSelect().Model(&readings)

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

	err := q.OrderExpr("timestamp DESC").Limit(limit).Scan(ctx)
	return readings, err
}
