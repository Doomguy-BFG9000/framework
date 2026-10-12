// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"fmt"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
)

// commandLine is one line a scenario runs and what the database and the output hold after it.
type commandLine struct {
	// before is a statement run first, empty for none.
	before string
	// stdin is what the line reads.
	stdin string
	// args is the line.
	args []string
	// check is a lookup that answers true after the line.
	check string
	// prints are the parts the output must name.
	prints []string
}

// runLine runs line through p on db and fails the test unless it exits 0 and leaves db as line says.
func (db testDB) runLine(p gonsole.Program, line commandLine) {
	db.t.Helper()
	if line.before != "" {
		db.run(line.before)
	}

	got := testkit.Run(db.t, p, line.stdin, line.args...)

	if got.Code != gonsole.ExitDone {
		db.t.Fatalf("%v = %d with %q, want %d", line.args, got.Code, got.Stderr, gonsole.ExitDone)
	}
	if !db.found(line.check) {
		db.t.Errorf("after %v the database fails %q, want it to hold", line.args, line.check)
	}
	wantPrinted(db.t, fmt.Sprint(line.args), got.Stdout, line.prints...)
	db.noSessionLeft()
}

func TestEveryAccountCommandRunsOnTheModulesStores(t *testing.T) {
	t.Parallel()

	db := empty(t)
	p := moduleProgram(db)
	asAdmin := []string{"-yes", "-as", "admin@example.com"}

	for _, line := range []commandLine{
		{args: []string{"migrate"}, check: "SELECT to_regclass('gonsole.records') IS NOT NULL",
			prints: []string{"migrated accounts", "migrated records"}},
		{stdin: demoPassword + "\n", args: []string{"account:create-admin", "-email", "admin@example.com",
			"-name", "Maria Perez", "-role", "admin"},
			check: "SELECT EXISTS (SELECT FROM auth.users WHERE email = 'admin@example.com' AND role = 'admin')"},
		{stdin: demoPassword + "\n", args: []string{"account:create-admin", "-email", "author@example.com",
			"-name", "Maria Perez", "-role", "author"},
			check: "SELECT EXISTS (SELECT FROM auth.users WHERE email = 'author@example.com' AND role = 'author')"},
		{before: "UPDATE auth.users SET role = '' WHERE email = 'author@example.com'",
			args:  append([]string{"account:grant-role", "-role", "editor"}, asAdmin...),
			check: "SELECT EXISTS (SELECT FROM auth.users WHERE email = 'author@example.com' AND role = 'editor')"},
		{args: append([]string{"account:role", "author@example.com", "author"}, asAdmin...),
			check: "SELECT EXISTS (SELECT FROM auth.users WHERE email = 'author@example.com' AND role = 'author')"},
		{args: append([]string{"account:disable", "author@example.com"}, asAdmin...),
			check: "SELECT EXISTS (SELECT FROM auth.users WHERE email = 'author@example.com' AND disabled)"},
		{args: append([]string{"account:enable", "author@example.com"}, asAdmin...),
			check: "SELECT EXISTS (SELECT FROM auth.users WHERE email = 'author@example.com' AND NOT disabled)"},
		{args: []string{"account:list"}, check: "SELECT count(*) = 2 FROM auth.users",
			prints: []string{"admin@example.com", "author@example.com"}},
		{args: []string{"account:records"}, check: "SELECT count(*) = 4 FROM gonsole.records",
			prints: []string{"account:grant-role", "account:role", "account:disable", "account:enable"}},
	} {
		db.runLine(p, line)
	}
}
