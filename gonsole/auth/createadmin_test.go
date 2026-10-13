// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/testkit"
)

// createPage is the help page of account:create-admin.
const createPage = `create an account under a role

Usage:
  myapp account:create-admin [flags]

Flags:
  -email address
    	address of the new account
  -name name
    	display name of the new account
  -role role
    	role the new account starts under
`

// migratedLine is the line the account schema step writes before a command that asks for it.
const migratedLine = "migrated accounts\n"

// creating returns the arguments that create maria.perez@example.com under role.
func creating(role string) []string {
	return []string{"account:create-admin", "-email", "maria.perez@example.com", "-name", "Maria Perez", "-role", role}
}

func TestCreateAdminCreatesAnAccountAfterTheSchema(t *testing.T) {
	t.Parallel()

	address := fresh(t)
	var reads rolesRead
	var before atomic.Int32
	before.Store(-1)
	cfg := config(&reads, "")
	cfg.Stores = func(ctx context.Context, call gonsole.Call) (auth.Stores, func(context.Context) error, error) {
		before.Store(worldAt(t, address).migrations.Load())
		return memoryStores(ctx, call)
	}

	got := testkit.Run(t, program(address, auth.CreateAdmin(cfg)), demoPassword+"\n", creating("admin")...)

	want := testkit.Result{Code: gonsole.ExitDone, Stdout: "Password: created user maria.perez@example.com\n",
		Stderr: migratedLine}
	if got != want {
		t.Fatalf("Run() = %+v, want %+v", got, want)
	}
	if ran := before.Load(); ran != 1 {
		t.Errorf("the accounts step ran %d times before the account, want once", ran)
	}
	if held := account(t, storeAt(t, address), "maria.perez@example.com"); held.Role != "admin" ||
		held.Name != "Maria Perez" {
		t.Errorf("account = %+v, want Maria Perez under admin", held)
	}
}

func TestCreateAdminRefusesALineItCannotRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		args   []string
		stdin  string
		code   int
		stdout string
		stderr string
		reads  int
	}{
		{"an unknown role", creating("owner"), "", gonsole.ExitMisused, "",
			migratedLine + "myapp: unknown role \"owner\", want admin, editor or author\n\n" + createPage, 1},
		{"a padded role", creating(" admin"), "", gonsole.ExitMisused, "",
			migratedLine + "myapp: unknown role \" admin\", want admin, editor or author\n\n" + createPage, 1},
		{"a taken address", []string{"account:create-admin", "-email", "admin@example.com", "-name", "Maria Perez",
			"-role", "admin"}, demoPassword + "\n", gonsole.ExitFailed, "Password: ",
			migratedLine + "myapp: gouncer: email already taken\n", 1},
		{"a weak password", creating("admin"), "short\n", gonsole.ExitFailed, "Password: ",
			migratedLine + "myapp: gouncer: password shorter than 12 characters\n", 1},
		{"a request for help", []string{"account:create-admin", "-h"}, "", gonsole.ExitDone, createPage, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var reads rolesRead

			got := testkit.Run(t, program(seeded(t), auth.CreateAdmin(config(&reads, ""))), tc.stdin, tc.args...)

			want := testkit.Result{Code: tc.code, Stdout: tc.stdout, Stderr: tc.stderr}
			if got != want {
				t.Errorf("Run() = %+v, want %+v", got, want)
			}
			if reads.count != tc.reads {
				t.Errorf("roles read %d times, want %d", reads.count, tc.reads)
			}
		})
	}
}

func TestCreateAdminRefusesAMissingFlagBeforeAnySchemaStep(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		args   []string
		stderr string
	}{
		{"no address", []string{"account:create-admin", "-name", "Maria Perez", "-role", "admin"},
			"myapp: account:create-admin wants -email <address>\n\n" + createPage},
		{"a blank address", []string{"account:create-admin", "-email", " ", "-name", "Maria Perez", "-role", "admin"},
			"myapp: account:create-admin wants -email <address>\n\n" + createPage},
		{"no name", []string{"account:create-admin", "-email", "maria.perez@example.com", "-role", "admin"},
			"myapp: account:create-admin wants -name <name>\n\n" + createPage},
		{"a blank name", append(creating("admin")[:3], "-name", "  ", "-role", "admin"),
			"myapp: account:create-admin wants -name <name>\n\n" + createPage},
		{"no role", creating("admin")[:5], "myapp: account:create-admin wants -role <role>\n\n" + createPage},
		{"a blank role", creating(" "), "myapp: account:create-admin wants -role <role>\n\n" + createPage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			address := fresh(t)
			var reads rolesRead

			got := testkit.Run(t, program(address, auth.CreateAdmin(config(&reads, ""))), "", tc.args...)

			if want := (testkit.Result{Code: gonsole.ExitMisused, Stderr: tc.stderr}); got != want {
				t.Errorf("Run() = %+v, want %+v", got, want)
			}
			if reads.count != 0 {
				t.Errorf("roles read %d times, want none", reads.count)
			}
			if ran := worldAt(t, address).migrations.Load(); ran != 0 {
				t.Errorf("the accounts step ran %d times, want a bare database", ran)
			}
		})
	}
}

func TestCreateAdminTakesARoleThePluginsDeclare(t *testing.T) {
	t.Parallel()

	address := migrated(t)
	cfg := auth.Config{Roles: func(context.Context, gonsole.Call) (auth.Roles, error) {
		return auth.Roles{Known: append(slices.Clone(vocabulary.Known), "superadmin"), Privileged: vocabulary.Privileged}, nil
	}, Stores: memoryStores}

	got := testkit.Run(t, program(address, auth.CreateAdmin(cfg)), demoPassword+"\n", creating("superadmin")...)

	if got.Code != gonsole.ExitDone {
		t.Fatalf("Run() = %+v, want a created account", got)
	}
	if held := account(t, storeAt(t, address), "maria.perez@example.com"); held.Role != "superadmin" {
		t.Errorf("role = %q, want superadmin", held.Role)
	}
}

func TestCreateAdminFailsWhenTheRolesCannotBeRead(t *testing.T) {
	t.Parallel()

	cfg := auth.Config{Roles: func(context.Context, gonsole.Call) (auth.Roles, error) {
		return auth.Roles{}, errors.New("the plugin table is locked")
	}, Stores: memoryStores}

	got := testkit.Run(t, program(migrated(t), auth.CreateAdmin(cfg)), demoPassword+"\n", creating("admin")...)

	want := testkit.Result{Code: gonsole.ExitFailed, Stderr: migratedLine + "myapp: the plugin table is locked\n"}
	if got != want {
		t.Errorf("Run() = %+v, want %+v", got, want)
	}
}

// unknownRole is one role account:create-admin refuses and the alternatives its refusal names.
type unknownRole struct {
	role, alternatives string
}

// refusesUnknownRole runs account:create-admin under refused.role over a program declaring known and wants it refused.
func refusesUnknownRole(t *testing.T, known []string, refused unknownRole) {
	t.Helper()
	cfg := auth.Config{Roles: func(context.Context, gonsole.Call) (auth.Roles, error) {
		return auth.Roles{Known: known}, nil
	}, Stores: memoryStores}

	got := testkit.Run(t, program(migrated(t), auth.CreateAdmin(cfg)), "", creating(refused.role)...)

	want := migratedLine + fmt.Sprintf("myapp: unknown role %q, want %s\n\n", refused.role, refused.alternatives) +
		createPage
	if got.Code != gonsole.ExitMisused || got.Stderr != want {
		t.Errorf("Run() = %+v, want exit 2 and %q", got, want)
	}
}

func TestCreateAdminNamesNoRoleWhenTheProgramDeclaresNone(t *testing.T) {
	t.Parallel()

	refusesUnknownRole(t, nil, unknownRole{"admin", "a role the program declares"})
}

func TestCreateAdminNamesTheOneRoleTheProgramDeclares(t *testing.T) {
	t.Parallel()

	refusesUnknownRole(t, []string{"admin"}, unknownRole{"owner", "admin"})
}
