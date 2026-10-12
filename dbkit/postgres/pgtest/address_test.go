// SPDX-License-Identifier: Apache-2.0

package pgtest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gopherium/framework/dbkit"
	"github.com/gopherium/framework/dbkit/postgres/pgtest"
)

const (
	// checkEmpty is the message of an empty or blank address.
	checkEmpty = "dbkit: refused database address: the address is empty or blank, " +
		"and pgx would read every setting from the PG* environment variables"
	// checkServer is a server address with the secret parts of the address tests, ready for its query.
	checkServer = "postgres://" + newSecretUser + "@" + newSecretHost + ":1/" + newSecretDatabase + "?"
)

// checkQueryKey returns the message of an address whose query sets the moving key.
func checkQueryKey(key string) string {
	return "dbkit: refused database address: the query sets " + key + ", give it before the ? instead"
}

// checkNoSecret fails t when message holds a part of an address the tests mark secret.
func checkNoSecret(t *testing.T, message string) {
	t.Helper()
	for _, secret := range []string{newSecret, newSecretUser, newSecretHost, newSecretDatabase} {
		if strings.Contains(message, secret) {
			t.Errorf("the message %q holds a secret part of the address, want none", message)
		}
	}
}

// checkRefused fails t unless CheckAddress refuses address with want, the message New stops a test with.
func checkRefused(t *testing.T, address, want string) {
	t.Helper()
	err := pgtest.CheckAddress(address)
	if err == nil {
		t.Fatalf("CheckAddress() = nil, want %q", want)
	}
	if !errors.Is(err, dbkit.ErrAddress) || err.Error() != want {
		t.Errorf("CheckAddress() = %v, want %q marked dbkit.ErrAddress", err, want)
	}
	checkNoSecret(t, err.Error())
	if stopped := newRun(t, address).message; stopped != err.Error() {
		t.Errorf("New() stopped the test with %q, want %q as CheckAddress answers", stopped, err.Error())
	}
}

func TestCheckAddressRefusesEveryAddressNewRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		address string
		want    string
	}{
		{"an empty address", "", checkEmpty},
		{"a blank address", " \t", checkEmpty},
		{"a bare at", "postgres://u:" + newSecret + "@ss@localhost:5434/postgres", newBareAt},
		{"a keyword and value address", "host=localhost user=u password=" + newSecret, newNotURL},
		{"an address with another scheme", "mysql://u:" + newSecret + "@localhost/postgres", newUnparsable},
		{"a URL that does not parse", "postgres://u:" + newSecret + "@localhost:port/postgres", newUnparsable},
		{"an IPv6 literal host", "postgres://u:" + newSecret + "@[::1]:5434/postgres", newColonHost},
		{"a list of hosts with ports", "postgres://u:" + newSecret + "@localhost:5434,localhost:5434/postgres",
			newColonHost},
		{"a query password", checkServer + "sslmode=disable&password=" + newSecret, newQueryPassword},
		{"a query password with an escaped key", checkServer + "sslmode=disable&pass%77ord=" + newSecret,
			newQueryPassword},
		{"a query sslpassword", checkServer + "sslmode=disable&sslpassword=" + newSecret, newQuerySSLPassword},
		{"a query password and a bad sslmode", checkServer + "password=" + newSecret + "&sslmode=disabled",
			newUnparsable},
		{"a query user", checkServer + "sslmode=disable&user=moved_user", checkQueryKey("user")},
		{"a query host", checkServer + "sslmode=disable&host=moved.example.com", checkQueryKey("host")},
		{"a query port", checkServer + "sslmode=disable&port=2", checkQueryKey("port")},
		{"a query dbname", checkServer + "sslmode=disable&dbname=moved_database", checkQueryKey("dbname")},
		{"a query database", checkServer + "sslmode=disable&database=moved_database", checkQueryKey("database")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			checkRefused(t, c.address, c.want)
		})
	}
}

func TestCheckAddressAcceptsAnAddressNewTakes(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"postgres://u:p@localhost:5434/postgres?sslmode=disable&application_name=kept_option",
		"postgresql://u:p@localhost:5434/postgres?sslmode=disable&application_name=kept_option",
	} {
		if err := pgtest.CheckAddress(address); err != nil {
			t.Errorf("CheckAddress() = %v, want nil for an address New takes", err)
		}
	}
}
