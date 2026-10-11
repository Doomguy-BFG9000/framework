// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

// testMigrate is how every test run of a schema step waits for the migration lock.
var testMigrate = sqlite.MigrateOptions{LockWait: 5 * time.Second, LockPoll: 10 * time.Millisecond}

// testOptions returns the handle options every test opens with.
func testOptions() dbsqlite.Options {
	return dbsqlite.Options{
		BusyTimeout: 5 * time.Second, CacheSize: 2048, MaxConns: 4, Synchronous: dbsqlite.SynchronousNormal,
	}
}

// steps returns the two schema steps in the order a program runs them.
func steps(opts sqlite.MigrateOptions) []gonsole.Step {
	return []gonsole.Step{sqlite.Migration(opts), sqlite.RecordMigration(opts)}
}

// applied runs both schema steps on db and fails t when one errs.
func applied(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, step := range steps(testMigrate) {
		if err := step.Apply(t.Context(), "", db); err != nil {
			t.Fatalf("applying the %s step: %v", step.Name, err)
		}
	}
}

// count returns the single integer lookup answers on db.
func count(t *testing.T, db *sql.DB, lookup string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&n); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	return n
}
