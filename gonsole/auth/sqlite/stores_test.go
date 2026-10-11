// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"errors"
	"testing"

	authkitsqlite "github.com/gopherium/gouncer/authkit/sqlite"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

var (
	_ auth.Accounts = (*authkitsqlite.UserStore)(nil)
	_               = auth.Config{Stores: sqlite.Stores}
)

func TestTheStoresRunOnTheCallsOneHandleWithNothingToRelease(t *testing.T) {
	t.Parallel()

	db := withRecords(t)

	stores, release, err := sqlite.Stores(t.Context(), gonsole.Call{}.WithDB(db))

	if err != nil || release != nil || stores.Accounts == nil || stores.Records == nil {
		t.Fatalf("Stores() = %v, a release %t, %v, want both stores and no release", stores, release != nil, err)
	}
	user, err := gouncer.NewUser("maria@example.com", "Maria Perez", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if err := stores.Accounts.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	held, err := stores.Records.Held(t.Context())
	accounts := count(t, db, "SELECT count(*) FROM auth_users")
	if err != nil || !held || accounts != 1 {
		t.Errorf("the stores read the records as held %t with %v and wrote %d accounts, want both on the handle",
			held, err, accounts)
	}
}

func TestTheStoresPassTheHandleFailureThroughUnchanged(t *testing.T) {
	t.Parallel()

	_, _, missing := sqlite.Stores(t.Context(), gonsole.Call{})
	_, _, describing := sqlite.Stores(t.Context(), gonsole.Call{Describe: true})

	if missing == nil || missing.Error() != "gonsole: no database handle in this call" ||
		!errors.Is(describing, gonsole.ErrDescribing) || describing != gonsole.ErrDescribing {
		t.Errorf("Stores() answered %v with no handle and %v for a describing call, want gonsole's errors as they are",
			missing, describing)
	}
}
