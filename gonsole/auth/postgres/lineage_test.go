// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
)

const (
	// versionsLookup lists the applied versions of both version tables on one line.
	versionsLookup = "SELECT (SELECT string_agg(version_id || ':' || is_applied, ',' ORDER BY id) " +
		"FROM auth.goose_db_version) || ' ' || (SELECT string_agg(version_id || ':' || is_applied, ',' ORDER BY id) " +
		"FROM gonsole.goose_db_version)"
	// plantRecord stores one record past every record store.
	plantRecord = "INSERT INTO gonsole.records (id, actor, command, args, flags) VALUES " +
		"('019a0000-0000-7000-8000-000000000001', 'admin@example.com', 'account:disable', '[]', '{}')"
	// oneRecord answers whether gonsole.records holds exactly one row.
	oneRecord = "SELECT count(*) = 1 FROM gonsole.records"
)

// text returns the single text lookup answers on db.
func (db testDB) text(lookup string) string {
	db.t.Helper()
	conn := db.dial()
	defer func() { _ = conn.Close(context.Background()) }()
	var value string
	if err := conn.QueryRow(db.t.Context(), lookup).Scan(&value); err != nil {
		db.t.Fatalf("%s: %v", lookup, err)
	}
	return value
}

func TestAProgramThatSwitchesStepsAppliesNothingTwice(t *testing.T) {
	t.Parallel()

	core := []gonsole.Step{auth.Migration(), auth.RecordMigration()}
	for _, tc := range []struct {
		name   string
		first  []gonsole.Step
		second []gonsole.Step
	}{
		{"from gonsole/auth to the module", core, steps()},
		{"from the module to gonsole/auth", steps(), core},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := empty(t)
			db.apply(tc.first...)
			db.run(plantRecord)
			before := db.text(versionsLookup)

			db.apply(tc.second...)

			if after := db.text(versionsLookup); after != before {
				t.Errorf("the version tables read %q after the other steps, want %q as before", after, before)
			}
			if !db.found(oneRecord) {
				t.Error("the planted record is gone after the other steps, want it kept")
			}
		})
	}
}
