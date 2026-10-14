// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
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
		{"a disabled account", "UPDATE auth.users SET disabled = true WHERE email = 'admin@example.com'",
			"admin@example.com", "myapp: the account admin@example.com is disabled\n"},
		{"an account never activated", "UPDATE auth.users SET confirmed = false WHERE email = 'admin@example.com'",
			"admin@example.com", "myapp: the account admin@example.com was never activated\n"},
		{"an account holding no role", "UPDATE auth.users SET role = '' WHERE email = 'author@example.com'",
			"author@example.com", "myapp: the account author@example.com holds no role, so it lacks manage_users\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := seeded(t)
			if tc.change != "" {
				db.run(tc.change)
			}

			got := testkit.Run(t, moduleProgram(db), "", "account:role", "author@example.com", "editor", "-yes",
				"-as", tc.actor)

			if got.Code != gonsole.ExitFailed || got.Stderr != tc.stderr {
				t.Errorf("account:role as %s = %d with %q, want %d with %q", tc.actor, got.Code, got.Stderr,
					gonsole.ExitFailed, tc.stderr)
			}
			if !db.found("SELECT count(*) = 0 FROM gonsole.records") {
				t.Errorf("account:role as %s left a record, want none", tc.actor)
			}
		})
	}
}

func TestAuthorizeLetsACapableAccountThrough(t *testing.T) {
	t.Parallel()

	db := seeded(t)

	got := testkit.Run(t, moduleProgram(db), "", "account:role", "author@example.com", "editor", "-yes",
		"-as", "admin@example.com")

	role := db.text("SELECT role FROM auth.users WHERE email = 'author@example.com'")
	if got.Code != gonsole.ExitDone || role != "editor" {
		t.Errorf("account:role as the admin = %d with %q and the role %q, want %d and editor", got.Code, got.Stderr,
			role, gonsole.ExitDone)
	}
}
