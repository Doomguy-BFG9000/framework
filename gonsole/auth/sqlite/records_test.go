// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

// recordID is the id of the record the store tests insert.
const recordID = "019a0000-0000-7000-8000-000000000001"

// roleChange returns an entry of an applied role change by the admin account.
func roleChange() auth.Entry {
	return auth.Entry{ID: recordID, Actor: "admin@example.com", Command: "account:role",
		Args: []string{"maria@example.com", "editor"}, Flags: map[string]string{"yes": "true"}}
}

// within reports whether micros falls between the microseconds of from and to.
func within(micros int64, from, to time.Time) bool {
	return micros >= from.UnixMicro() && micros <= to.UnixMicro()
}

func TestALimitBelowZeroListsNoRecords(t *testing.T) {
	t.Parallel()

	store := sqlite.NewRecordStore(withRecords(t))
	mustInsert(t, store, roleChange())

	listed, err := store.Latest(t.Context(), -1)

	if err != nil {
		t.Fatalf("Latest(-1) error = %v, want nil", err)
	}
	if listed == nil || len(listed) != 0 {
		t.Errorf("Latest(-1) = %#v, want an empty list, at most -1 records", listed)
	}
}

func TestARecordKeepsItsTimeAsUTCMicrosecondsSinceTheEpoch(t *testing.T) {
	t.Parallel()

	db := withRecords(t)
	store := sqlite.NewRecordStore(db)
	before := time.Now()
	mustInsert(t, store, roleChange())
	after := time.Now()

	kind := text(t, db, "SELECT typeof(applied_at) FROM gonsole_records")
	var stored int64
	lookup := "SELECT applied_at FROM gonsole_records"
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&stored); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	listed, err := store.Latest(t.Context(), 1)
	if err != nil || len(listed) != 1 {
		t.Fatalf("Latest() = %v, %v, want the one record", listed, err)
	}
	if kind != "integer" || !within(stored, before, after) {
		t.Errorf("applied_at is %s %d, want an integer between %d and %d", kind, stored, before.UnixMicro(),
			after.UnixMicro())
	}
	if got := listed[0].AppliedAt; got.Location() != time.UTC || got.UnixMicro() != stored {
		t.Errorf("Latest() lists the record at %v, want %d microseconds in UTC", got, stored)
	}
}
