// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

package sqlite_test

import (
	"database/sql"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

// migrationLock opens the migration lock file beside the main file of db.
func migrationLock(t *testing.T, db *sql.DB) *os.File {
	t.Helper()
	var path string
	lookup := "SELECT file FROM pragma_database_list WHERE name = 'main'"
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&path); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	lock, err := os.OpenFile(path+".migrate.lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("opening the migration lock: %v", err)
	}
	return lock
}

// lockFree reports whether the migration lock beside the main file of db can be taken now, then lets it go.
func lockFree(t *testing.T, db *sql.DB) bool {
	t.Helper()
	lock := migrationLock(t, db)
	defer func() { _ = lock.Close() }()
	return syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
}

func TestStepsRunOneAfterTheOther(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())

	var events []string
	for _, step := range steps(testMigrate) {
		accounts := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name = 'auth_users'") == 1
		events = append(events, fmt.Sprintf("%s started with the accounts table %t", step.Name, accounts))
		if err := step.Apply(t.Context(), "", db); err != nil {
			t.Fatalf("applying the %s step: %v", step.Name, err)
		}
		events = append(events, fmt.Sprintf("%s ended with the lock free %t", step.Name, lockFree(t, db)))
	}

	want := []string{"accounts started with the accounts table false", "accounts ended with the lock free true",
		"records started with the accounts table true", "records ended with the lock free true"}
	if !slices.Equal(events, want) {
		t.Errorf("the steps ran as %q, want %q", events, want)
	}
}

func TestEachStepGivesUpOnAMigrationLockHeldPastItsWait(t *testing.T) {
	t.Parallel()

	short := sqlite.MigrateOptions{LockWait: 100 * time.Millisecond, LockPoll: 10 * time.Millisecond}
	for _, step := range steps(short) {
		t.Run(step.Name, func(t *testing.T) {
			t.Parallel()
			db := sqlitetest.Open(t, testOptions())
			lock := migrationLock(t, db)
			t.Cleanup(func() { _ = lock.Close() })
			if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
				t.Fatalf("holding the migration lock: %v", err)
			}

			start := time.Now()
			err := step.Apply(t.Context(), "", db)
			waited := time.Since(start)

			versions := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name LIKE '%goose_db_version'")
			if err == nil || !strings.Contains(err.Error(), "migration lock") || versions != 0 {
				t.Errorf("the %s step behind a held lock = %v with %d version tables, want the lock named and none",
					step.Name, err, versions)
			}
			if waited < short.LockWait {
				t.Errorf("the %s step gave up after %v, want it to wait out the lock wait of %v", step.Name, waited,
					short.LockWait)
			}
		})
	}
}
