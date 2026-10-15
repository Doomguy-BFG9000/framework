// SPDX-License-Identifier: Apache-2.0

package dbkit

import (
	"context"
	"database/sql"
	"errors"
)

// InTx runs body in a write transaction on db, committing only when body returns nil and rolling back otherwise.
func InTx(ctx context.Context, db *sql.DB, body func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := body(tx); err != nil {
		return rollbackAfter(err, tx)
	}
	if err := ended(ctx, tx.Commit()); err != nil {
		return rollbackAfter(err, tx)
	}
	return nil
}

// rollbackAfter rolls tx back and returns err, joined with the rollback error when the rollback fails.
func rollbackAfter(err error, tx *sql.Tx) error {
	if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		return errors.Join(err, rollbackErr)
	}
	return err
}
