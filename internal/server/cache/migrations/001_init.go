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
			device_name VARCHAR,
			point_name VARCHAR,
			data_type VARCHAR,
			bool_val BOOLEAN,
			num_val DOUBLE,
			unit VARCHAR,
			timestamp BIGINT NOT NULL,
			device_hash VARCHAR
		)`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS readings`)
		return err
	})
}
