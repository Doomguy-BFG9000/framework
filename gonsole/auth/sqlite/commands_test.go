// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
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

	opts := testOptions()
	db, err := dbsqlite.Open("sqlite:"+path, opts)
	if err != nil {
		t.Fatalf("opening the file the run made: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	admins := count(t, db, "SELECT count(*) FROM auth_users WHERE email = 'maria.perez@example.com' AND role = 'admin'")
	if got.Code != gonsole.ExitDone || admins != 1 || opens.Load() != 1 {
		t.Errorf("account:create-admin on a fresh file = %d with %q, %d admins and %d opens, want %d, 1 and 1",
			got.Code, got.Stderr, admins, opens.Load(), gonsole.ExitDone)
	}
}

func TestEveryAccountCommandRunsOnTheOneHandle(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, check, want string
		args              []string
	}{
		{"account:grant-role", "SELECT count(*) FROM auth_users WHERE role = ''", "0",
			[]string{"account:grant-role", "-role", "author", "-yes", "-as", "admin@example.com"}},
		{"account:list", "SELECT count(*) FROM auth_users", "3", []string{"account:list"}},
		{"account:role", "SELECT role FROM auth_users WHERE email = 'author@example.com'", "editor",
			[]string{"account:role", "author@example.com", "editor", "-yes", "-as", "admin@example.com"}},
		{"account:disable", "SELECT disabled FROM auth_users WHERE email = 'author@example.com'", "1",
			[]string{"account:disable", "author@example.com", "-yes", "-as", "admin@example.com"}},
		{"account:enable", "SELECT disabled FROM auth_users WHERE email = 'editor@example.com'", "0",
			[]string{"account:enable", "editor@example.com", "-yes", "-as", "admin@example.com"}},
		{"account:records", "SELECT count(*) FROM gonsole_records", "0", []string{"account:records"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, db := seeded(t)
			var opens atomic.Int32

			got := testkit.Run(t, program(path, &opens), "", tc.args...)

			if after := text(t, db, tc.check); got.Code != gonsole.ExitDone || after != tc.want || opens.Load() != 1 {
				t.Errorf("%v = %d with %q, %q and %d opens, want %d, %q and 1", tc.args, got.Code, got.Stderr, after,
					opens.Load(), gonsole.ExitDone, tc.want)
			}
		})
	}
}
