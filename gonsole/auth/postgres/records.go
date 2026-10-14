// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbpostgres "github.com/gopherium/framework/dbkit/postgres"
	"github.com/gopherium/framework/gonsole/auth"
)

// heldRecords reads whether the database holds the gonsole.records table.
const heldRecords = `SELECT to_regclass('gonsole.records') IS NOT NULL`

// storeRecord inserts one record, finding the account the acting address names.
const storeRecord = `INSERT INTO gonsole.records (id, actor, account_id, command, args, flags)
VALUES ($1::uuid, $2, (SELECT id FROM auth.users WHERE email = $2), $3, $4::jsonb, $5::jsonb)`

// latestRecords reads the newest records first, as many as the limit allows.
const latestRecords = `SELECT id::text, applied_at, actor, account_id::text, command, args, flags
FROM gonsole.records ORDER BY applied_at DESC, id DESC LIMIT $1`

// recordStore is the record store of the gonsole.records table in a program's PostgreSQL database.
type recordStore struct {
	// pool runs the statements.
	pool *pgxpool.Pool
}

// NewRecordStore returns the record store of the PostgreSQL database behind pool.
func NewRecordStore(pool *pgxpool.Pool) auth.RecordStore {
	return recordStore{pool: pool}
}

// Held reports whether the database holds the gonsole.records table.
func (r recordStore) Held(ctx context.Context) (bool, error) {
	var held bool
	if err := r.pool.QueryRow(ctx, heldRecords).Scan(&held); err != nil {
		return false, failed("check the records", err)
	}
	return held, nil
}

// Insert stores entry as applied now on the server's clock, finding the account its actor names in auth.users.
func (r recordStore) Insert(ctx context.Context, entry auth.Entry) error {
	args, flags := encoded(entry)
	_, err := r.pool.Exec(ctx, storeRecord, entry.ID, entry.Actor, entry.Command, args, flags)
	return failed("insert a record", err)
}

// Latest returns the newest entries first, at most limit of them, and an empty list for none or a limit below zero.
func (r recordStore) Latest(ctx context.Context, limit int) ([]auth.Entry, error) {
	rows, err := r.pool.Query(ctx, latestRecords, max(limit, 0))
	if err != nil {
		return nil, failed("list the records", err)
	}
	held, err := pgx.CollectRows(rows, scanned)
	if err != nil {
		return nil, failed("list the records", err)
	}
	return held, nil
}

// scanned returns the entry of row in UTC, its arguments and flags decoded from their JSON.
func scanned(row pgx.CollectableRow) (auth.Entry, error) {
	var entry auth.Entry
	err := row.Scan(&entry.ID, &entry.AppliedAt, &entry.Actor, &entry.AccountID, &entry.Command, &entry.Args,
		&entry.Flags)
	entry.AppliedAt = entry.AppliedAt.UTC()
	return entry, err
}

// encoded returns the JSON text of the entry's arguments and flags, an empty list and object for none.
func encoded(entry auth.Entry) (string, string) {
	flags := map[string]string{}
	maps.Copy(flags, entry.Flags)
	args, _ := json.Marshal(append([]string{}, entry.Args...))
	object, _ := json.Marshal(flags)
	return string(args), string(object)
}

// failed returns nil for a nil err, or err marked with its dbkit class under action.
func failed(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("postgres: %s: %w", action, dbpostgres.Classify(err))
}
