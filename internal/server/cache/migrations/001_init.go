package migrations

import (
	"context"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

var Migrations = migrate.NewMigrations()

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS readings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sn VARCHAR NOT NULL,
			upload_time BIGINT NOT NULL,
			device VARCHAR(16) NOT NULL,
			device_name VARCHAR,
			point VARCHAR(16) NOT NULL,
			point_name VARCHAR,
			data_type VARCHAR NOT NULL,
			value TEXT NOT NULL,
			quality INTEGER NOT NULL DEFAULT 0,
			unit VARCHAR,
			timestamp BIGINT NOT NULL
		)`)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_readings_sn_ts ON readings(sn, timestamp)`)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_readings_dev_pt ON readings(device, point)`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS readings`)
		return err
	})
}
