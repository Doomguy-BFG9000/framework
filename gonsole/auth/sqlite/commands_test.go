// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
)

func TestTheFirstAdminComesOnAFreshFileThroughTheOneHandle(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "site.db")
	var opens atomic.Int32

	got := testkit.Run(t, program(path, &opens), demoPassword+"\n", "account:create-admin",
		"-email", "maria.perez@example.com", "-name", "Maria Perez", "-role", "admin")

	if got.Code != gonsole.ExitDone || opens.Load() != 1 {
		t.Fatalf("account:create-admin on a fresh file = %d with %q and %d opens, want %d and 1",
			got.Code, got.Stderr, opens.Load(), gonsole.ExitDone)
	}
	db, err := dbsqlite.Open("sqlite:"+path, testOptions())
	if err != nil {
		t.Fatalf("opening the file the run made: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	admins := count(t, db, "SELECT count(*) FROM auth_users WHERE email = 'maria.perez@example.com' AND role = 'admin'")
	if admins != 1 {
		t.Errorf("account:create-admin on a fresh file left %d admins, want 1", admins)
	}
}

func TestEveryAccountCommandRunsOnTheOneHandle(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, before, check, want string
		args, prints              []string
	}{
		{"account:grant-role", "UPDATE auth_users SET role = '' WHERE email = 'author@example.com'",
			"SELECT role FROM auth_users WHERE email = 'author@example.com'", "author",
			[]string{"account:grant-role", "-role", "author", "-yes", "-as", "admin@example.com"}, nil},
		{"account:list", "", "SELECT count(*) FROM gonsole_records", "0", []string{"account:list"},
			[]string{"admin@example.com", "editor@example.com", "author@example.com"}},
		{"account:role", "", "SELECT role FROM auth_users WHERE email = 'author@example.com'", "editor",
			[]string{"account:role", "author@example.com", "editor", "-yes", "-as", "admin@example.com"}, nil},
		{"account:disable", "", "SELECT disabled FROM auth_users WHERE email = 'author@example.com'", "1",
			[]string{"account:disable", "author@example.com", "-yes", "-as", "admin@example.com"}, nil},
		{"account:enable", "UPDATE auth_users SET disabled = 1 WHERE email = 'editor@example.com'",
			"SELECT disabled FROM auth_users WHERE email = 'editor@example.com'", "0",
			[]string{"account:enable", "editor@example.com", "-yes", "-as", "admin@example.com"}, nil},
		{"account:records", "INSERT INTO gonsole_records (id, applied_at, actor, command, args, flags) VALUES " +
			"('019a0000-0000-7000-8000-000000000001', 1767225600000000, 'admin@example.com', 'account:disable', " +
			"'[\"author@example.com\"]', '{}')", "SELECT count(*) FROM gonsole_records", "1",
			[]string{"account:records"}, []string{"admin@example.com", "account:disable", "author@example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, db := seeded(t)
			if tc.before != "" {
				mustExec(t, db, tc.before)
			}
			var opens atomic.Int32

			got := testkit.Run(t, program(path, &opens), "", tc.args...)

			if got.Code != gonsole.ExitDone || opens.Load() != 1 {
				t.Errorf("%v = %d with %q and %d opens, want %d and 1", tc.args, got.Code, got.Stderr, opens.Load(),
					gonsole.ExitDone)
			}
			if after := text(t, db, tc.check); after != tc.want {
				t.Errorf("after %v the file reads %q, want %q", tc.args, after, tc.want)
			}
			wantPrinted(t, fmt.Sprint(tc.args), got.Stdout, tc.prints...)
		})
	}
}
