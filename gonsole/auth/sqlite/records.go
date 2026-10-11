// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/gopherium/framework/dbkit"
	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/gonsole/auth"
)

// heldRecords reads whether the file holds the gonsole_records table.
const heldRecords = `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'gonsole_records')`

// storeRecord inserts one record, finding the account the acting address names.
const storeRecord = `INSERT INTO gonsole_records (id, actor, account_id, command, args, flags, applied_at)
VALUES (?1, ?2, (SELECT id FROM auth_users WHERE email = ?2), ?3, ?4, ?5, ?6)`

// latestRecords reads the newest records first, as many as the limit allows.
const latestRecords = `SELECT id, applied_at, actor, account_id, command, args, flags
FROM gonsole_records ORDER BY applied_at DESC, id DESC LIMIT ?1`

// recordStore is the record store of the gonsole_records table in a program's SQLite file.
type recordStore struct {
	// db is the run's one handle.
	db *sql.DB
}

// NewRecordStore returns the record store of the SQLite file behind db.
func NewRecordStore(db *sql.DB) auth.RecordStore {
	return recordStore{db: db}
}

// Held reports whether the file holds the gonsole_records table.
func (r recordStore) Held(ctx context.Context) (bool, error) {
	var held bool
	if err := r.db.QueryRowContext(ctx, heldRecords).Scan(&held); err != nil {
		return false, failed("check the records", err)
	}
	return held, nil
}

// Insert stores entry as applied now, finding the account its actor names in auth_users.
func (r recordStore) Insert(ctx context.Context, entry auth.Entry) error {
	return r.insert(ctx, entry, time.Now())
}

// insert stores entry as applied at appliedAt, finding the account its actor names in auth_users.
func (r recordStore) insert(ctx context.Context, entry auth.Entry, appliedAt time.Time) error {
	args, flags := encoded(entry)
	_, err := r.db.ExecContext(ctx, storeRecord, entry.ID, entry.Actor, entry.Command, args, flags,
		dbkit.Time{Time: appliedAt})
	return failed("insert a record", err)
}

// Latest returns the newest entries first, at most limit of them, and an empty list for none.
func (r recordStore) Latest(ctx context.Context, limit int) ([]auth.Entry, error) {
	rows, err := r.db.QueryContext(ctx, latestRecords, limit)
	if err != nil {
		return nil, failed("list the records", err)
	}
	held, err := collected(rows)
	if err != nil {
		return nil, failed("list the records", err)
	}
	return held, nil
}

// collected returns the entries of rows in their order, an empty list for none, and closes rows.
func collected(rows *sql.Rows) ([]auth.Entry, error) {
	defer func() { _ = rows.Close() }()
	held := []auth.Entry{}
	for rows.Next() {
		entry, err := scanned(rows)
		if err != nil {
			return nil, err
		}
		held = append(held, entry)
	}
	return held, rows.Err()
}

// scanned returns the entry of the current row, its arguments and flags decoded from their JSON text.
func scanned(rows *sql.Rows) (auth.Entry, error) {
	var (
		entry       auth.Entry
		appliedAt   dbkit.Time
		args, flags string
	)
	err := rows.Scan(&entry.ID, &appliedAt, &entry.Actor, &entry.AccountID, &entry.Command, &args, &flags)
	if err != nil {
		return auth.Entry{}, err
	}
	entry.AppliedAt = appliedAt.Time
	err = errors.Join(json.Unmarshal([]byte(args), &entry.Args), json.Unmarshal([]byte(flags), &entry.Flags))
	if err != nil {
		return auth.Entry{}, fmt.Errorf("decode the record %s: %w", entry.ID, err)
	}
	return entry, nil
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
	return fmt.Errorf("sqlite: %s: %w", action, dbsqlite.Classify(err))
}
