// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	authkitpostgres "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	dbpostgres "github.com/gopherium/framework/dbkit/postgres"
	"github.com/gopherium/framework/gonsole"
)

// migrationFiles holds the migrations of the command records.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// recordsVersionTable keeps the lineage of the records migrations inside their own schema.
const recordsVersionTable = "gonsole.goose_db_version"

// errUnreadable refuses an address pgx cannot parse, and holds none of pgx's text.
var errUnreadable = errors.New("postgres: open the database: pgx cannot parse the address or one of its settings")

// migrationSource holds the records migrations at its root.
var migrationSource = mustSub(migrationFiles, "migrations")

// Migration returns the step that applies gouncer's account schema.
func Migration() gonsole.Step {
	return gonsole.Step{Name: "accounts", Run: authkitpostgres.Migrate}
}

// RecordMigration returns the step that applies the schema keeping the command records.
func RecordMigration() gonsole.Step {
	return gonsole.Step{Name: "records", Run: migrateRecords}
}

// migrateRecords applies the records migrations to the database at databaseURL.
func migrateRecords(ctx context.Context, databaseURL string) error {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return errUnreadable
	}
	db := stdlib.OpenDB(*config)
	defer func() { _ = db.Close() }()
	err = dbpostgres.Migrate(ctx, db, dbpostgres.Migrations{Table: recordsVersionTable, FS: migrationSource})
	if err != nil {
		return fmt.Errorf("postgres: apply the records schema: %w", dbpostgres.Classify(err))
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
