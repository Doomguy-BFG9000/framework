// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/postgres"
	"github.com/gopherium/framework/gonsole/testkit"
)

// vocabulary is the role vocabulary of the test program, where only an admin may manage users.
var vocabulary = auth.Roles{
	Known: []string{"admin", "editor", "author"}, Privileged: gouncer.Roles{"admin"},
	Capabilities: map[string][]string{"admin": {"manage_users"}},
}

// moduleProgram returns a program called myapp over db that runs on the module's stores and schema steps.
func moduleProgram(db testDB) gonsole.Program {
	cfg := auth.Config{
		Roles:      func(context.Context, gonsole.Call) (auth.Roles, error) { return vocabulary, nil },
		Capability: "manage_users", RecordTimeout: 5 * time.Second, RecordsLimit: 10, Stores: postgres.Stores,
	}
	settings := map[string]string{"MYAPP_DATABASE_URL": db.address}
	return gonsole.Program{
		Name:       "myapp",
		Env:        gonsole.Env{Prefix: "MYAPP_", Getenv: testkit.Getenv(settings)},
		Database:   "DATABASE_URL",
		Migrations: steps(),
		Commands:   append(auth.Commands(cfg), auth.Records(cfg)),
		Authorize:  auth.Authorize(cfg),
		Record:     auth.Record(cfg),
	}
}

// wantPrinted fails t unless out, which command printed, names every part.
func wantPrinted(t *testing.T, command, out string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(out, part) {
			t.Errorf("%s printed %q, want it to name %s", command, out, part)
		}
	}
}
