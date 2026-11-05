package store

import (
	"context"
	"database/sql"
	"errors"
)

// GetMeta reads one app_meta value: small app state that isn't a user
// setting, such as the clip server's port (R18). ok is false when the key
// was never set.
func (d *DB) GetMeta(ctx context.Context, key string) (value string, ok bool, err error) {
	err = d.Reader().QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetMeta writes one app_meta value, replacing any earlier one.
func (d *DB) SetMeta(ctx context.Context, key, value string) error {
	return d.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO app_meta (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
		return err
	})
}
