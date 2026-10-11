// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"database/sql"
	"io"
	"sync/atomic"
	"testing"
	"time"

	authkitsqlite "github.com/gopherium/gouncer/authkit/sqlite"

	"github.com/gopherium/gouncer"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/sqlite"
	"github.com/gopherium/framework/gonsole/testkit"
)

// demoPassword is the password the seeded accounts log in with.
const demoPassword = "correct horse battery"

// vocabulary is the role vocabulary of the test program, where only an admin may manage users.
var vocabulary = auth.Roles{
	Known: []string{"admin", "editor", "author"}, Privileged: gouncer.Roles{"admin"},
	Capabilities: map[string][]string{"admin": {"manage_users"}},
}

// testConfig returns the account commands' configuration on the run's one SQLite handle.
func testConfig() auth.Config {
	return auth.Config{
		Roles:      func(context.Context, gonsole.Call) (auth.Roles, error) { return vocabulary, nil },
		Capability: "manage_users", RecordTimeout: 5 * time.Second, RecordsLimit: 10, Stores: sqlite.Stores,
	}
}

// program returns a program called myapp over the SQLite file at path, counting each open of its handle in opens.
func program(path string, opens *atomic.Int32) gonsole.Program {
	cfg := testConfig()
	settings := map[string]string{"MYAPP_DATABASE_URL": "sqlite:" + path}
	return gonsole.Program{
		Name:     "myapp",
		Env:      gonsole.Env{Prefix: "MYAPP_", Getenv: testkit.Getenv(settings)},
		Database: "DATABASE_URL",
		Open: func(_ context.Context, address string) (*sql.DB, error) {
			opens.Add(1)
			opts := testOptions()
			opts.Create = true
			return dbsqlite.Open(address, opts)
		},
		Migrations: steps(testMigrate),
		Commands:   append(auth.Commands(cfg), auth.Records(cfg)),
		Authorize:  auth.Authorize(cfg),
		Record:     auth.Record(cfg),
	}
}

// pathOf returns the path of the main file behind db.
func pathOf(t *testing.T, db *sql.DB) string {
	t.Helper()
	var path string
	lookup := "SELECT file FROM pragma_database_list WHERE name = 'main'"
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&path); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	return path
}

// seeded returns the path of a migrated file holding one enabled account per role, and the test's own handle on it.
func seeded(t *testing.T) (string, *sql.DB) {
	t.Helper()
	db := withRecords(t)
	var accounts []auth.Account
	for _, role := range vocabulary.Known {
		accounts = append(accounts, auth.Account{
			Email: role + "@example.com", Name: role + " account", Password: demoPassword, Role: role,
		})
	}
	if err := auth.EnsureAccounts(t.Context(), authkitsqlite.NewUserStore(db), accounts, io.Discard); err != nil {
		t.Fatalf("seeding the accounts: %v", err)
	}
	return pathOf(t, db), db
}

// text returns the single text lookup answers on db.
func text(t *testing.T, db *sql.DB, lookup string) string {
	t.Helper()
	var value string
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&value); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	return value
}
