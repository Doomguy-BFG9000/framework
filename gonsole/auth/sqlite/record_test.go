// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
)

func TestAnAppliedCommandRecordsOneRowNamingTheAccount(t *testing.T) {
	t.Parallel()

	path, db := seeded(t)
	var opens atomic.Int32

	got := testkit.Run(t, program(path, &opens), "", "account:role", "author@example.com", "editor", "-yes",
		"-as", " Admin@Example.com ")

	admin := text(t, db, "SELECT id FROM auth_users WHERE email = 'admin@example.com'")
	recorded := text(t, db, "SELECT actor || ' ' || command || ' ' || account_id FROM gonsole_records")
	want := "admin@example.com account:role " + admin
	if got.Code != gonsole.ExitDone || count(t, db, "SELECT count(*) FROM gonsole_records") != 1 || recorded != want {
		t.Errorf("an applied account:role = %d with %q and the record %q, want %d and %q", got.Code, got.Stderr,
			recorded, gonsole.ExitDone, want)
	}
}

func TestAccountRecordsListsTheRecordInTextAndInUTCJSON(t *testing.T) {
	t.Parallel()

	path, _ := seeded(t)
	var opens atomic.Int32
	applied := testkit.Run(t, program(path, &opens), "", "account:role", "author@example.com", "editor", "-yes",
		"-as", "admin@example.com")
	if applied.Code != gonsole.ExitDone {
		t.Fatalf("account:role = %d with %q, want %d", applied.Code, applied.Stderr, gonsole.ExitDone)
	}

	listed := testkit.Run(t, program(path, &opens), "", "account:records")
	asJSON := testkit.Run(t, program(path, &opens), "", "account:records", "-json")

	var document struct {
		Records []struct {
			AppliedAt string `json:"applied_at"`
			Actor     string `json:"actor"`
			Command   string `json:"command"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(asJSON.Stdout), &document); err != nil || len(document.Records) != 1 {
		t.Fatalf("account:records -json printed %q, %v, want one record", asJSON.Stdout, err)
	}
	first := document.Records[0]
	if !strings.Contains(listed.Stdout, "admin@example.com") || !strings.Contains(listed.Stdout, "account:role") ||
		first.Actor != "admin@example.com" || first.Command != "account:role" || !strings.HasSuffix(first.AppliedAt, "Z") {
		t.Errorf("account:records printed %q and -json %+v, want the admin's role change at a UTC time",
			listed.Stdout, first)
	}
}

func TestAccountRecordsPrintsAnEmptyListOnAnEmptyTable(t *testing.T) {
	t.Parallel()

	path := pathOf(t, withRecords(t))
	var opens atomic.Int32

	got := testkit.Run(t, program(path, &opens), "", "account:records", "-json")

	if got.Code != gonsole.ExitDone || !strings.Contains(got.Stdout, `"records": []`) {
		t.Errorf("account:records -json on an empty table = %d with %q, want an empty list", got.Code, got.Stdout)
	}
}
