package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS devices (
			id BIGSERIAL PRIMARY KEY,
			sn VARCHAR NOT NULL,
			device_hash VARCHAR(16) NOT NULL,
			device_name VARCHAR,
			type VARCHAR,
			first_seen BIGINT NOT NULL,
			last_seen BIGINT NOT NULL,
			status VARCHAR NOT NULL DEFAULT 'unknown',
			UNIQUE(sn, device_hash)
		)`)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_devices_sn ON devices(sn)`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS devices`)
		return err
	})
}
