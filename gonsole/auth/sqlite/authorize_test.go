// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"sync/atomic"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
)

func TestAuthorizeRefusesAnAccountItCannotLetThrough(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, change, actor, stderr string
	}{
		{"a role that lacks the capability", "", "editor@example.com",
			"myapp: the account editor@example.com holds the role editor, which lacks manage_users\n"},
		{"a disabled account", "UPDATE auth_users SET disabled = 1 WHERE email = 'admin@example.com'",
			"admin@example.com", "myapp: the account admin@example.com is disabled\n"},
		{"an account never activated", "UPDATE auth_users SET confirmed = 0 WHERE email = 'admin@example.com'",
			"admin@example.com", "myapp: the account admin@example.com was never activated\n"},
		{"an account holding no role", "UPDATE auth_users SET role = '' WHERE email = 'author@example.com'",
			"author@example.com", "myapp: the account author@example.com holds no role, so it lacks manage_users\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, db := seeded(t)
			if tc.change != "" {
				mustExec(t, db, tc.change)
			}
			var opens atomic.Int32

			got := testkit.Run(t, program(path, &opens), "", "account:role", "author@example.com", "editor", "-yes",
				"-as", tc.actor)

			records := count(t, db, "SELECT count(*) FROM gonsole_records")
			if got.Code != gonsole.ExitFailed || got.Stderr != tc.stderr || records != 0 {
				t.Errorf("account:role as %s = %d with %q and %d records, want %d with %q and none", tc.actor,
					got.Code, got.Stderr, records, gonsole.ExitFailed, tc.stderr)
			}
		})
	}
}

func TestAuthorizeLetsACapableAccountThrough(t *testing.T) {
	t.Parallel()

	path, db := seeded(t)
	var opens atomic.Int32

	got := testkit.Run(t, program(path, &opens), "", "account:role", "author@example.com", "editor", "-yes",
		"-as", "admin@example.com")

	role := text(t, db, "SELECT role FROM auth_users WHERE email = 'author@example.com'")
	if got.Code != gonsole.ExitDone || role != "editor" {
		t.Errorf("account:role as the admin = %d with %q and the role %q, want %d and editor", got.Code, got.Stderr,
			role, gonsole.ExitDone)
	}
}
