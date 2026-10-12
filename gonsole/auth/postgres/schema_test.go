// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	authkitpostgres "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopherium/framework/dbkit"
	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth/postgres"
)

// steps returns the two schema steps of the module in the order a program runs them.
func steps() []gonsole.Step {
	return []gonsole.Step{postgres.Migration(), postgres.RecordMigration()}
}

// apply runs each step on db, failing the test at the first that fails.
func (db testDB) apply(steps ...gonsole.Step) {
	db.t.Helper()
	for _, step := range steps {
		if err := step.Run(db.t.Context(), db.address); err != nil {
			db.t.Fatalf("running the %s step: %v", step.Name, err)
		}
	}
}

func TestMigrationAppliesTheAccountSchemaAsOftenAsItRuns(t *testing.T) {
	t.Parallel()

	db := empty(t)
	step := postgres.Migration()

	db.apply(step, step)

	if step.Name != "accounts" {
		t.Errorf("Name = %q, want accounts", step.Name)
	}
	if _, err := authkitpostgres.NewUserStore(db.pool()).ListUsers(t.Context()); err != nil {
		t.Errorf("ListUsers() after the migration = %v, want the account schema in place", err)
	}
}

func TestRecordMigrationCreatesTheRecordsAsOftenAsItRuns(t *testing.T) {
	t.Parallel()

	db := empty(t)
	step := postgres.RecordMigration()

	db.apply(step, step)

	if step.Name != "records" {
		t.Errorf("Name = %q, want records", step.Name)
	}
	if !db.exists("gonsole.records") || !db.exists("gonsole.goose_db_version") {
		t.Errorf("gonsole.records and its version table are missing after the migration")
	}
}

func TestBothStepsKeepTheirOwnVersionTables(t *testing.T) {
	t.Parallel()

	db := empty(t)

	db.apply(steps()...)

	if !db.exists("auth.goose_db_version") || !db.exists("gonsole.goose_db_version") {
		t.Errorf("the steps left auth.goose_db_version %t and gonsole.goose_db_version %t, want both",
			db.exists("auth.goose_db_version"), db.exists("gonsole.goose_db_version"))
	}
	if !db.found("SELECT max(version_id) = 1 FROM gonsole.goose_db_version") {
		t.Error("gonsole.goose_db_version holds no version 1 as its highest, want the one records migration")
	}
}

func TestEveryStepWaitsForTheMigrationLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		step  gonsole.Step
		table string
	}{
		{postgres.Migration(), "auth.users"},
		{postgres.RecordMigration(), "gonsole.records"},
	}
	for _, tt := range tests {
		t.Run(tt.step.Name, func(t *testing.T) {
			t.Parallel()

			db := empty(t)
			db.holdMigrationLock()
			ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()

			err := tt.step.Run(ctx, db.address)

			if !errors.Is(err, context.DeadlineExceeded) || db.exists(tt.table) {
				t.Errorf("Run() = %v, want the deadline and no %s table", err, tt.table)
			}
		})
	}
}

func TestEveryStepLetsRunsAtOnceAllSucceed(t *testing.T) {
	t.Parallel()

	for _, step := range steps() {
		t.Run(step.Name, func(t *testing.T) {
			t.Parallel()

			db := empty(t)
			failures := make(chan error, 4)
			var runs sync.WaitGroup
			for range 4 {
				runs.Go(func() { failures <- step.Run(t.Context(), db.address) })
			}
			runs.Wait()
			close(failures)

			var all []error
			for err := range failures {
				all = append(all, err)
			}
			if err := errors.Join(all...); err != nil {
				t.Errorf("Run() = %v, want every run to succeed", err)
			}
		})
	}
}

func TestEveryStepSucceedsWhenAnotherSessionCreatesItsSchemaFirst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		step   gonsole.Step
		schema string
	}{
		{postgres.Migration(), "auth"},
		{postgres.RecordMigration(), "gonsole"},
	}
	for _, tt := range tests {
		t.Run(tt.step.Name, func(t *testing.T) {
			t.Parallel()

			db := empty(t)
			holder := db.connect()
			tx, err := holder.Begin(t.Context())
			if err != nil {
				t.Fatalf("beginning: %v", err)
			}
			if _, err := tx.Exec(t.Context(), "CREATE SCHEMA "+tt.schema); err != nil {
				t.Fatalf("creating the %s schema: %v", tt.schema, err)
			}
			ran := make(chan error, 1)
			go func() { ran <- tt.step.Run(t.Context(), db.address) }()
			db.awaitSession(holder, ran, waitingOnLock)

			if err := tx.Commit(t.Context()); err != nil {
				t.Fatalf("committing the %s schema: %v", tt.schema, err)
			}

			if err := <-ran; err != nil {
				t.Errorf("Run() = %v, want it to succeed", err)
			}
		})
	}
}

func TestEveryStepLetsAConcurrentIndexBuildFinishWhileItWaits(t *testing.T) {
	t.Parallel()

	for _, step := range steps() {
		t.Run(step.Name, func(t *testing.T) {
			t.Parallel()

			db := empty(t)
			db.run("CREATE TABLE items (x int)")
			holder := db.holdMigrationLock()
			ran := make(chan error, 1)
			go func() { ran <- step.Run(t.Context(), db.address) }()
			db.awaitSession(holder, ran, waitingOnAdvisory)

			_, built := holder.Exec(t.Context(), "CREATE INDEX CONCURRENTLY items_x ON items (x)")
			if _, err := holder.Exec(t.Context(), lockRelease, lock.DefaultLockID); err != nil {
				t.Fatalf("releasing the migration lock: %v", err)
			}

			if err := <-ran; built != nil || err != nil {
				t.Errorf("index build = %v and Run() = %v, want both to finish", built, err)
			}
		})
	}
}

func TestRecordMigrationNamesEveryFailureUnderItsAction(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		address func(t *testing.T) string
		want    string
	}{
		{"an address it cannot read", func(*testing.T) string { return "postgres://%zz" },
			"postgres: open the database: "},
		{"a database it cannot reach", func(*testing.T) string {
			return "postgres://postgres@127.0.0.1:1/none?connect_timeout=1"
		}, "postgres: apply the records schema: "},
		{"a step it cannot apply", func(t *testing.T) string {
			db := empty(t)
			db.run("CREATE SCHEMA gonsole", "CREATE TABLE gonsole.records (id int)")
			return db.address
		}, "postgres: apply the records schema: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := postgres.RecordMigration().Run(t.Context(), tc.address(t))

			if !strings.HasPrefix(fmt.Sprint(err), tc.want) {
				t.Errorf("Run() = %v, want the failure under %q", err, tc.want)
			}
		})
	}
}

func TestRecordMigrationNamesAnUnreadableAddressWithoutItsPassword(t *testing.T) {
	t.Parallel()

	err := postgres.RecordMigration().Run(t.Context(), "postgres://u:"+newSecret+"/tail@localhost:5434/app")

	message := fmt.Sprint(err)
	if !strings.HasPrefix(message, "postgres: open the database: ") || strings.Contains(message, newSecret) {
		t.Errorf("Run() = %v, want the database named and no part of the password", err)
	}
}

func TestTheRecordsStepHeldPastItsLockTimeoutAnswersDbkitsBusyClass(t *testing.T) {
	t.Parallel()

	db := recorded(t)
	holder := db.connect()
	lockVersions := "BEGIN; LOCK TABLE gonsole.goose_db_version IN ACCESS EXCLUSIVE MODE"
	if _, err := holder.Exec(t.Context(), lockVersions); err != nil {
		t.Fatalf("locking the version table: %v", err)
	}

	err := postgres.RecordMigration().Run(t.Context(), db.withQuery("lock_timeout", "100ms").address)

	if !errors.Is(err, dbkit.ErrBusy) || !strings.HasPrefix(fmt.Sprint(err), "postgres: apply the records schema: ") {
		t.Errorf("the records step behind a held version table = %v, want dbkit.ErrBusy under the step", err)
	}
}

func TestRecordMigrationNamesTheSchemaItCannotCreate(t *testing.T) {
	t.Parallel()

	db := empty(t)
	db.run("DO $$ BEGIN EXECUTE format(" +
		"'ALTER DATABASE %I SET default_transaction_read_only = on', current_database()); END $$")

	err := postgres.RecordMigration().Run(t.Context(), db.address)

	message := fmt.Sprint(err)
	if !strings.HasPrefix(message, "postgres: apply the records schema: ") ||
		!strings.Contains(message, "dbkit: create the schema gonsole: ") {
		t.Errorf("Run() = %v, want the gonsole schema named under the records schema", err)
	}
}
