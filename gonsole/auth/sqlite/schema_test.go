// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit"
	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

// insertRecord stores a record row with the given values for the columns that have no default.
const insertRecord = "INSERT INTO gonsole_records (id, applied_at, actor, command, args, flags) " +
	"VALUES (?, ?, ?, ?, ?, ?)"

func TestBothStepsMigrateTheirOwnTablesUnderTheirOwnNames(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())

	var names []string
	for _, step := range steps(testMigrate) {
		names = append(names, step.Name)
	}
	applied(t, db)
	applied(t, db)

	tables := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name IN ('auth_users', 'gonsole_records')")
	versions := count(t, db, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN "+
		"('auth_goose_db_version', 'gonsole_goose_db_version')")
	records := count(t, db, "SELECT max(version_id) FROM gonsole_goose_db_version")
	if !slices.Equal(names, []string{"accounts", "records"}) || tables != 2 || versions != 2 || records != 1 {
		t.Errorf("steps %v left %d of the 2 tables and %d of the 2 version tables, records at %d, "+
			"want accounts then records, both tables, both version tables and records at 1", names, tables,
			versions, records)
	}
}

func TestTheRecordsStepRefusesLockSettingsAtZero(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())

	err := sqlite.RecordMigration(sqlite.MigrateOptions{}).Apply(t.Context(), "", db)

	tables := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name = 'gonsole_records'")
	if err == nil || !strings.HasPrefix(err.Error(), "sqlite: apply the records schema: ") || tables != 0 {
		t.Errorf("the records step with no lock settings = %v with %d records tables, want a refusal and none",
			err, tables)
	}
}

func TestTheRecordsStepHeldPastTheBusyTimeoutAnswersDbkitsBusyClass(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.BusyTimeout = 50 * time.Millisecond
	db := sqlitetest.Open(t, opts)
	writer, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("beginning the writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Rollback() })
	if _, err := writer.ExecContext(t.Context(), "CREATE TABLE core_items (id TEXT NOT NULL PRIMARY KEY)"); err != nil {
		t.Fatalf("writing inside the writer: %v", err)
	}

	err = sqlite.RecordMigration(testMigrate).Apply(context.Background(), "", db)

	if !errors.Is(err, dbkit.ErrBusy) || !strings.HasPrefix(err.Error(), "sqlite: apply the records schema: ") {
		t.Errorf("the records step behind a write past the busy timeout = %v, want dbkit.ErrBusy under the step", err)
	}
}

func TestTheRecordsTableRefusesNullsAndInvalidJSON(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())
	applied(t, db)
	valid := []any{"019a0000-0000-7000-8000-000000000001", int64(1767225600000000), "admin@example.com",
		"account:role", "[]", "{}"}

	for column := range valid {
		row := slices.Clone(valid)
		row[column] = nil
		_, err := db.ExecContext(t.Context(), insertRecord, row...)
		if got := dbsqlite.Classify(err); !errors.Is(got, dbkit.ErrNotNull) {
			t.Errorf("a record with column %d NULL = %v, want dbkit.ErrNotNull", column, got)
		}
	}
	for _, column := range []int{4, 5} {
		row := slices.Clone(valid)
		row[column] = "not json"
		_, err := db.ExecContext(t.Context(), insertRecord, row...)
		if got := dbsqlite.Classify(err); !errors.Is(got, dbkit.ErrCheck) {
			t.Errorf("a record with column %d holding invalid JSON = %v, want dbkit.ErrCheck", column, got)
		}
	}
}

func TestTheRecordsIndexCarriesItsName(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())
	applied(t, db)

	var definition string
	lookup := "SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'gonsole_records_applied_at'"
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&definition); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	if !strings.HasSuffix(definition, "(applied_at DESC)") {
		t.Errorf("the records index reads %q, want it on applied_at DESC", definition)
	}
}
