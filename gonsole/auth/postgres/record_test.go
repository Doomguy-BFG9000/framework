// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	authkitpostgres "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/testkit"
)

// recordBound is how long a record behind a held table may take before the test gives up on its timeout.
const recordBound = 5 * time.Second

// seeded returns a fresh database holding the records table and one enabled account per role of vocabulary.
func seeded(t *testing.T) testDB {
	t.Helper()
	db := recorded(t)
	pool := db.pool()
	defer pool.Close()
	var accounts []auth.Account
	for _, role := range vocabulary.Known {
		accounts = append(accounts, auth.Account{
			Email: role + "@example.com", Name: role + " account", Password: demoPassword, Role: role,
		})
	}
	if err := auth.EnsureAccounts(t.Context(), authkitpostgres.NewUserStore(pool), accounts, io.Discard); err != nil {
		t.Fatalf("seeding the accounts: %v", err)
	}
	return db
}

// purge returns a guarded command named report:purge that writes nothing and prints done.
func purge() gonsole.Command {
	return gonsole.Command{
		Name: "report:purge", Summary: "do nothing", Capability: "manage_users",
		Run: func(_ context.Context, call gonsole.Call) error {
			_, err := fmt.Fprintln(call.Stdout, "done")
			return err
		},
	}
}

func TestRecordEndsAtItsTimeout(t *testing.T) {
	t.Parallel()

	db := seeded(t)
	holder := db.connect()
	if _, err := holder.Exec(t.Context(), "BEGIN; LOCK TABLE gonsole.records IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatalf("locking the records: %v", err)
	}
	p := moduleProgram(db)
	p.Env.Getenv = testkit.Getenv(map[string]string{
		"MYAPP_DATABASE_URL": db.address, "MYAPP_COMMAND_RECORD_TIMEOUT": "200ms",
	})
	p.Commands = append(p.Commands, purge())
	ran := make(chan testkit.Result, 1)

	go func() { ran <- testkit.Run(t, p, "", "report:purge", "-as", "admin@example.com") }()

	select {
	case got := <-ran:
		if got.Code != gonsole.ExitFailed || !strings.HasPrefix(got.Stderr, "myapp: record report:purge: ") {
			t.Errorf("report:purge = %d with %q, want %d naming the record", got.Code, got.Stderr, gonsole.ExitFailed)
		}
	case <-time.After(recordBound):
		_ = holder.Close(context.Background())
		<-ran
		t.Fatalf("the record outlived its timeout of 200ms by %v, want it to end at the timeout", recordBound)
	}
}
