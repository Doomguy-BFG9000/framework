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

func TestALimitBelowZeroListsNoRecords(t *testing.T) {
	t.Parallel()

	store := sqlite.NewRecordStore(withRecords(t))
	if err := store.Insert(t.Context(), roleChange()); err != nil {
		t.Fatalf("Insert() error = %v, want nil", err)
	}

	listed, err := store.Latest(t.Context(), -1)

	if err != nil || listed == nil || len(listed) != 0 {
		t.Errorf("Latest(-1) = %v, %v, want an empty list, at most -1 records", listed, err)
	}
}

func TestARecordKeepsItsTimeAsUTCMicrosecondsSinceTheEpoch(t *testing.T) {
	t.Parallel()

	db := withRecords(t)
	store := sqlite.NewRecordStore(db)
	before := time.Now()
	if err := store.Insert(t.Context(), roleChange()); err != nil {
		t.Fatalf("Insert() error = %v, want nil", err)
	}
	after := time.Now()

	var kind string
	var stored int64
	lookup := "SELECT typeof(applied_at), applied_at FROM gonsole_records"
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&kind, &stored); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	listed, err := store.Latest(t.Context(), 1)
	if err != nil || len(listed) != 1 {
		t.Fatalf("Latest() = %v, %v, want the one record", listed, err)
	}
	if kind != "integer" || stored < before.UnixMicro() || stored > after.UnixMicro() {
		t.Errorf("applied_at is %s %d, want an integer between %d and %d", kind, stored, before.UnixMicro(),
			after.UnixMicro())
	}
	if got := listed[0].AppliedAt; got.Location() != time.UTC || got.UnixMicro() != stored {
		t.Errorf("Latest() lists the record at %v, want %d microseconds in UTC", got, stored)
	}
}
