package migrations

import (
	"context"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

var Migrations = migrate.NewMigrations()

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS cached_readings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device VARCHAR(16) NOT NULL,
			device_name VARCHAR,
			point VARCHAR(16) NOT NULL,
			point_name VARCHAR,
			data_type VARCHAR NOT NULL,
			value TEXT NOT NULL,
			quality INTEGER NOT NULL DEFAULT 0,
			unit VARCHAR,
			timestamp BIGINT NOT NULL,
			status INTEGER DEFAULT 0
		)`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS readings`)
		return err
	})
}
