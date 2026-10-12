// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gopherium/framework/dbkit"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/postgres"
)

// badArgs stores a record whose arguments are a JSON object, past the record store.
const badArgs = "INSERT INTO gonsole.records (id, actor, command, args, flags) VALUES " +
	"('019a0000-0000-7000-8000-000000000001', 'admin@example.com', 'account:role', '{}', '{}')"

// wantUnder fails t unless err is a failure under the action the store names.
func wantUnder(t *testing.T, err error, action string) {
	t.Helper()
	if prefix := "postgres: " + action + ": "; !strings.HasPrefix(fmt.Sprint(err), prefix) {
		t.Errorf("the call answered %v, want a failure under %q", err, prefix)
	}
}

func TestARepeatedRecordIDAnswersDbkitsUniqueClass(t *testing.T) {
	t.Parallel()

	store := postgres.NewRecordStore(recorded(t).pool())
	mustInsert(t, store, roleChange())

	err := store.Insert(t.Context(), roleChange())

	wantUnder(t, err, "insert a record")
	if !errors.Is(err, dbkit.ErrUnique) {
		t.Errorf("Insert() of a repeated id = %v, want dbkit.ErrUnique", err)
	}
}

func TestHeldOnAClosedPoolFailsUnderItsAction(t *testing.T) {
	t.Parallel()

	pool := recorded(t).pool()
	pool.Close()

	_, err := postgres.NewRecordStore(pool).Held(t.Context())

	wantUnder(t, err, "check the records")
}

func TestEveryCallOnAMissingTableFailsUnderItsAction(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		action string
		call   func(ctx context.Context, store auth.RecordStore) error
	}{
		{"insert a record", func(ctx context.Context, store auth.RecordStore) error {
			return store.Insert(ctx, roleChange())
		}},
		{"list the records", func(ctx context.Context, store auth.RecordStore) error {
			_, err := store.Latest(ctx, 10)
			return err
		}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			db := recorded(t)
			db.run("DROP TABLE gonsole.records")

			err := tc.call(t.Context(), postgres.NewRecordStore(db.pool()))

			wantUnder(t, err, tc.action)
		})
	}
}

func TestARecordThatListsBadlyFailsTheWholeList(t *testing.T) {
	t.Parallel()

	db := recorded(t)
	db.run(badArgs)

	listed, err := postgres.NewRecordStore(db.pool()).Latest(t.Context(), 10)

	wantUnder(t, err, "list the records")
	if listed != nil {
		t.Errorf("Latest() over a record with object arguments = %v, want no list", listed)
	}
}
