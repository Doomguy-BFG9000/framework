// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	authkitsqlite "github.com/gopherium/gouncer/authkit/sqlite"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/gonsole"
)

// migrationFiles holds the migrations of the command records.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// recordsVersionTable is the goose version table of the records migrations, beside any other owner's in the file.
const recordsVersionTable = "gonsole_goose_db_version"

// migrationSource holds the records migrations at its root.
var migrationSource = mustSub(migrationFiles, "migrations")

// MigrateOptions sets how a run of a schema step waits for the migration lock of the database file.
type MigrateOptions struct {
	// LockWait is how long a run waits for the migration lock, above zero.
	LockWait time.Duration
	// LockPoll is how often a waiting run tries the migration lock, above zero.
	LockPoll time.Duration
}

// Migration returns the step that applies gouncer's account tables on the run's one handle.
func Migration(opts MigrateOptions) gonsole.Step {
	lock := authkitsqlite.MigrateOptions{LockWait: opts.LockWait, LockPoll: opts.LockPoll}
	return gonsole.Step{Name: "accounts", RunOn: func(ctx context.Context, db *sql.DB) error {
		return authkitsqlite.Migrate(ctx, db, lock)
	}}
}

// RecordMigration returns the step that applies the table keeping the command records on the run's one handle.
func RecordMigration(opts MigrateOptions) gonsole.Step {
	return gonsole.Step{Name: "records", RunOn: func(ctx context.Context, db *sql.DB) error {
		return migrateRecords(ctx, db, opts)
	}}
}

// migrateRecords applies the records migrations to db under the migration lock of its file.
func migrateRecords(ctx context.Context, db *sql.DB, opts MigrateOptions) error {
	err := dbsqlite.Migrate(ctx, db, dbsqlite.Migrations{
		Table: recordsVersionTable, FS: migrationSource, LockWait: opts.LockWait, LockPoll: opts.LockPoll,
	})
	if err != nil {
		return fmt.Errorf("sqlite: apply the records schema: %w", dbsqlite.Classify(err))
	}
	return nil
}

// mustSub returns the subtree of fsys at dir, and panics when dir is not a valid path.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
