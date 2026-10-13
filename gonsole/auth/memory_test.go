// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
)

// The errors a world's stores answer when a test makes them fail.
var (
	errLookup  = errors.New("look up the account: the accounts are missing")
	errListing = errors.New("list users: the accounts are missing")
	errGrant   = errors.New("grant role: the store refused the write")
	errHeld    = errors.New("look for the records: the store is out of reach")
	errRead    = errors.New("read the records: the flags are missing")
)

// world is one in-memory database: an account store, a records fake and how often the accounts step ran.
type world struct {
	accounts   *memoryAccounts
	records    *memoryRecords
	migrations atomic.Int32
}

// stores returns the world's account store and records.
func (w *world) stores() auth.Stores {
	return auth.Stores{Accounts: w.accounts, Records: w.records}
}

// memoryAccounts is gouncer's in-memory account store, whose grant answers grantErr when it is set.
type memoryAccounts struct {
	*testkit.Store
	grantErr error
}

// GrantRoleToRoleless answers the end of ctx or grantErr first, the grant of the store otherwise.
func (a *memoryAccounts) GrantRoleToRoleless(ctx context.Context, role string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.grantErr != nil {
		return 0, a.grantErr
	}
	return a.Store.GrantRoleToRoleless(ctx, role)
}

// memoryRecords is a record store in memory, finding the account of each entry in its world's account store.
type memoryRecords struct {
	mu        sync.Mutex
	accounts  *memoryAccounts
	held      bool
	rows      []auth.Entry
	heldErr   error
	latestErr error
}

// Held reports whether the records step ran, answering heldErr when it is set.
func (r *memoryRecords) Held(context.Context) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held, r.heldErr
}

// Insert stores entry as applied now.
func (r *memoryRecords) Insert(ctx context.Context, entry auth.Entry) error {
	return r.plant(ctx, entry, time.Now().UTC())
}

// Latest lists at most limit entries, the newest first and the higher id first among entries applied at once.
func (r *memoryRecords) Latest(_ context.Context, limit int) ([]auth.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.latestErr != nil {
		return nil, r.latestErr
	}
	listed := slices.SortedFunc(slices.Values(r.rows), newestFirst)
	return append([]auth.Entry{}, listed[:min(max(limit, 0), len(listed))]...), nil
}

// plant stores entry as applied at appliedAt, under the account its actor names and with lists that are never nil.
func (r *memoryRecords) plant(ctx context.Context, entry auth.Entry, appliedAt time.Time) error {
	account, err := r.accountOf(ctx, entry.Actor)
	if err != nil {
		return err
	}
	flags := map[string]string{}
	maps.Copy(flags, entry.Flags)
	entry.AppliedAt, entry.AccountID = appliedAt, account
	entry.Args, entry.Flags = append([]string{}, entry.Args...), flags
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, entry)
	return nil
}

// accountOf returns the id of the account at actor, nil when no account answers to it.
func (r *memoryRecords) accountOf(ctx context.Context, actor string) (*string, error) {
	user, err := r.accounts.UserByEmail(ctx, actor)
	if errors.Is(err, gouncer.ErrUserNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id := user.ID.String()
	return &id, nil
}

// hold makes the records held, as their step does.
func (r *memoryRecords) hold() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.held = true
}

// all returns every entry, the oldest first.
func (r *memoryRecords) all() []auth.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.SortedStableFunc(slices.Values(r.rows), func(a, b auth.Entry) int {
		return a.AppliedAt.Compare(b.AppliedAt)
	})
}

// newestFirst orders a before b when a was applied later, or at once under a higher id.
func newestFirst(a, b auth.Entry) int {
	return cmp.Or(b.AppliedAt.Compare(a.AppliedAt), strings.Compare(b.ID, a.ID))
}

// worlds holds every live world by its address.
var worlds sync.Map

// addresses counts the worlds made so far, so each takes an address of its own.
var addresses atomic.Int64

// fresh registers an empty world under an address of its own, dropped when the test ends, and returns the address.
func fresh(t *testing.T) string {
	t.Helper()
	address := fmt.Sprintf("memory://%d", addresses.Add(1))
	accounts := &memoryAccounts{Store: testkit.NewStore()}
	worlds.Store(address, &world{accounts: accounts, records: &memoryRecords{accounts: accounts}})
	t.Cleanup(func() { worlds.Delete(address) })
	return address
}

// lookup returns the world at address, an error naming an address no world holds.
func lookup(address string) (*world, error) {
	held, ok := worlds.Load(address)
	if !ok {
		return nil, fmt.Errorf("no world at %s", address)
	}
	return held.(*world), nil
}

// worldAt returns the world at address, failing the test when no world is there.
func worldAt(t *testing.T, address string) *world {
	t.Helper()
	held, err := lookup(address)
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// storeAt returns the account store of the world at address.
func storeAt(t *testing.T, address string) *memoryAccounts {
	t.Helper()
	return worldAt(t, address).accounts
}

// migrated returns the address of a fresh world the accounts step ran on.
func migrated(t *testing.T) string {
	t.Helper()
	address := fresh(t)
	if err := accountsStep.Run(t.Context(), address); err != nil {
		t.Fatalf("applying the account schema: %v", err)
	}
	return address
}

// memoryStores builds the stores of the world at the call's database setting, with nothing to release.
func memoryStores(_ context.Context, call gonsole.Call) (auth.Stores, func(context.Context) error, error) {
	address, err := call.DatabaseURL()
	if err != nil {
		return auth.Stores{}, nil, err
	}
	held, err := lookup(address)
	if err != nil {
		return auth.Stores{}, nil, err
	}
	return held.stores(), nil, nil
}

// step returns the step called name that applies apply to the world at the address it runs against.
func step(name string, apply func(w *world)) gonsole.Step {
	return gonsole.Step{Name: name, Run: func(_ context.Context, address string) error {
		held, err := lookup(address)
		if err == nil {
			apply(held)
		}
		return err
	}}
}

// accountsStep is the accounts step, counting its runs in the world at its address.
var accountsStep = step("accounts", func(w *world) { w.migrations.Add(1) })

// recordsStep is the records step, which makes the records of the world at its address held.
var recordsStep = step("records", func(w *world) { w.records.hold() })

// change is one write a test makes to an account store before its run.
type change func(t *testing.T, store *memoryAccounts)

// run applies each change to the account store of the world at address.
func run(t *testing.T, address string, changes ...change) {
	t.Helper()
	store := storeAt(t, address)
	for _, apply := range changes {
		apply(t, store)
	}
}

// holding returns the change that puts the account at email under role.
func holding(email, role string) change {
	return func(t *testing.T, store *memoryAccounts) {
		t.Helper()
		if err := store.SetUserRole(t.Context(), account(t, store, email).ID, role, nil); err != nil {
			t.Fatalf("SetUserRole(%s, %q) = %v", email, role, err)
		}
	}
}

// disabling returns the change that disables the account at email.
func disabling(email string) change {
	return func(t *testing.T, store *memoryAccounts) {
		t.Helper()
		if err := store.SetUserDisabled(t.Context(), account(t, store, email).ID, true); err != nil {
			t.Fatalf("SetUserDisabled(%s) = %v", email, err)
		}
	}
}

// inviting returns the change that adds an account at email that was invited and never activated.
func inviting(email string) change {
	return func(t *testing.T, store *memoryAccounts) {
		t.Helper()
		invited, err := gouncer.NewInvitedUser(email, "Maria Perez")
		if err != nil {
			t.Fatalf("NewInvitedUser() = %v", err)
		}
		if err := store.CreateUser(t.Context(), invited); err != nil {
			t.Fatalf("CreateUser() = %v", err)
		}
	}
}
