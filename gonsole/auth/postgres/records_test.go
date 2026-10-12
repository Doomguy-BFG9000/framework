// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"fmt"
	"testing"

	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/postgres"
)

// recordID is the id of the record the store tests insert.
const recordID = "019a0000-0000-7000-8000-000000000001"

// roleChange returns an entry of an applied role change by the admin account.
func roleChange() auth.Entry {
	return auth.Entry{ID: recordID, Actor: "admin@example.com", Command: "account:role",
		Args: []string{"maria@example.com", "editor"}, Flags: map[string]string{"yes": "true"}}
}

// mustInsert inserts entry through store and fails t when it errs.
func mustInsert(t *testing.T, store auth.RecordStore, entry auth.Entry) {
	t.Helper()
	if err := store.Insert(t.Context(), entry); err != nil {
		t.Fatalf("Insert() error = %v, want nil", err)
	}
}

func TestALimitBelowZeroListsNoRecords(t *testing.T) {
	t.Parallel()

	store := postgres.NewRecordStore(recorded(t).pool())
	mustInsert(t, store, roleChange())

	listed, err := store.Latest(t.Context(), -1)

	if err != nil {
		t.Fatalf("Latest(-1) error = %v, want nil", err)
	}
	if listed == nil || len(listed) != 0 {
		t.Errorf("Latest(-1) = %#v, want an empty list", listed)
	}
}

func TestARecordKeepsTheServersTimeOfItsInsert(t *testing.T) {
	t.Parallel()

	db := recorded(t)
	store := postgres.NewRecordStore(db.pool())
	before := db.text("SELECT now()::text")

	mustInsert(t, store, roleChange())

	if !db.found("SELECT applied_at BETWEEN $1::timestamptz AND now() FROM gonsole.records", before) {
		t.Errorf("the record's applied_at falls outside the server's clock from %s to now, want the time of its insert",
			before)
	}
}

func TestAnEntryListsBackUnderEveryQueryMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"exec", "simple_protocol"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			store := postgres.NewRecordStore(recorded(t).withQuery("default_query_exec_mode", mode).pool())
			mustInsert(t, store, roleChange())

			listed, err := store.Latest(t.Context(), 10)

			if err != nil || len(listed) != 1 {
				t.Fatalf("Latest() = %v, %v, want the one record", listed, err)
			}
			got, want := listed[0], roleChange()
			if fmt.Sprint(got.ID, got.Actor, got.Command, got.Args, got.Flags) !=
				fmt.Sprint(want.ID, want.Actor, want.Command, want.Args, want.Flags) {
				t.Errorf("Latest() lists %+v, want the role change %+v whole", got, want)
			}
		})
	}
}
