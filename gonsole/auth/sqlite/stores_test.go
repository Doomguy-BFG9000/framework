// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
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

func TestTheStoresComeWithNothingToRelease(t *testing.T) {
	t.Parallel()

	stores, release, err := sqlite.Stores(t.Context(), gonsole.Call{}.WithDB(withRecords(t)))

	if err != nil || release != nil {
		t.Fatalf("Stores() = a release %t, %v, want no release and no error", release != nil, err)
	}
	if stores.Accounts == nil || stores.Records == nil {
		t.Errorf("Stores() = %+v, want both stores", stores)
	}
}

func TestTheStoresRunOnTheCallsOneHandle(t *testing.T) {
	t.Parallel()

	db := withRecords(t)

	stores, _, err := sqlite.Stores(t.Context(), gonsole.Call{}.WithDB(db))

	if err != nil {
		t.Fatalf("Stores() error = %v, want nil", err)
	}
	user, err := gouncer.NewUser("maria@example.com", "Maria Perez", demoPassword)
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if err := stores.Accounts.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	if accounts := count(t, db, "SELECT count(*) FROM auth_users"); accounts != 1 {
		t.Errorf("the account store wrote %d accounts on the handle, want 1", accounts)
	}
	if held, err := stores.Records.Held(t.Context()); err != nil || !held {
		t.Errorf("the record store read the records as held %t with %v, want held on the handle", held, err)
	}
}

func TestTheStoresPassTheHandleFailureThroughUnchanged(t *testing.T) {
	t.Parallel()

	_, gonsoles := gonsole.Call{}.DB(t.Context())
	if gonsoles == nil {
		t.Fatal("gonsole.Call{}.DB() error = nil, want the failure of a call with no handle")
	}

	_, _, missing := sqlite.Stores(t.Context(), gonsole.Call{})
	_, _, describing := sqlite.Stores(t.Context(), gonsole.Call{Describe: true})

	if missing == nil || missing.Error() != gonsoles.Error() {
		t.Errorf("Stores() with no handle = %v, want %v as it is", missing, gonsoles)
	}
	if describing != gonsole.ErrDescribing {
		t.Errorf("Stores() for a describing call = %v, want %v as it is", describing, gonsole.ErrDescribing)
	}
}
