// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gopherium/framework/dbkit"
	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

// errPlanted is the failure a chosen statement answers with.
var errPlanted = errors.New("planted failure")

// wantUnder fails t unless err matches want under the action the store names.
func wantUnder(t *testing.T, err, want error, action string) {
	t.Helper()
	if !errors.Is(err, want) || !strings.HasPrefix(err.Error(), "sqlite: "+action+": ") {
		t.Errorf("the call answered %v, want %v under %q", err, want, "sqlite: "+action+": ")
	}
}

func TestARepeatedRecordIDAnswersDbkitsUniqueClass(t *testing.T) {
	t.Parallel()

	store := sqlite.NewRecordStore(withRecords(t))
	if err := store.Insert(t.Context(), roleChange()); err != nil {
		t.Fatalf("Insert() error = %v, want nil", err)
	}

	err := store.Insert(t.Context(), roleChange())

	wantUnder(t, err, dbkit.ErrUnique, "insert a record")
}

func TestEveryStatementFailureReachesTheCallerUnderItsAction(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		statement, action string
		call              func(ctx context.Context, store auth.RecordStore) error
	}{
		{sqlite.HeldRecords, "check the records", func(ctx context.Context, store auth.RecordStore) error {
			_, err := store.Held(ctx)
			return err
		}},
		{sqlite.StoreRecord, "insert a record", func(ctx context.Context, store auth.RecordStore) error {
			return store.Insert(ctx, roleChange())
		}},
		{sqlite.LatestRecords, "list the records", func(ctx context.Context, store auth.RecordStore) error {
			_, err := store.Latest(ctx, 10)
			return err
		}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			faults := &sqlitetest.Faults{}
			db := template.OpenWithFaults(t, faults)
			if err := sqlite.RecordMigration(testMigrate).Apply(t.Context(), "", db); err != nil {
				t.Fatalf("applying the records step: %v", err)
			}
			faults.FailStatement(tc.statement, errPlanted)

			err := tc.call(t.Context(), sqlite.NewRecordStore(db))

			wantUnder(t, err, errPlanted, tc.action)
		})
	}
}

func TestARecordThatListsBadlyFailsTheWholeList(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		appliedAt any
		args      string
	}{
		{"arguments that are a JSON object", int64(1767225600000000), "{}"},
		{"a time that is no integer", "yesterday", "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := withRecords(t)
			mustExec(t, db, insertRecord, recordID, tc.appliedAt, "admin@example.com", "account:role", tc.args, "{}")

			listed, err := sqlite.NewRecordStore(db).Latest(t.Context(), 10)

			if listed != nil || err == nil || !strings.HasPrefix(err.Error(), "sqlite: list the records: ") {
				t.Errorf("Latest() over a record with %s = %v, %v, want no list and a failure under the action",
					tc.name, listed, err)
			}
		})
	}
}
