// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/postgres"
	"github.com/gopherium/framework/gonsole/testkit"
)

var _ = auth.Config{Stores: postgres.Stores}

// storesProbe records what the stores of one call answered before and after their release.
type storesProbe struct {
	// held is what Held answered before the release.
	held bool
	// before is the error Held answered before the release.
	before error
	// after is the error Held answered after the release.
	after error
}

// command returns a command named probe that builds the stores of its call and fills the probe.
func (probe *storesProbe) command() gonsole.Command {
	return gonsole.Command{Name: "probe", Summary: "build the stores", Run: probe.run}
}

// run builds the stores of call, asks Held before and after their release and keeps both answers.
func (probe *storesProbe) run(ctx context.Context, call gonsole.Call) error {
	stores, release, err := postgres.Stores(ctx, call)
	if err != nil {
		return err
	}
	probe.held, probe.before = stores.Records.Held(ctx)
	if err := release(ctx); err != nil {
		return err
	}
	_, probe.after = stores.Records.Held(ctx)
	return nil
}

func TestTheStoresRunOnAPoolOfTheDatabaseSettingThatTheReleaseCloses(t *testing.T) {
	t.Parallel()

	db := recorded(t)
	var probe storesProbe
	p := moduleProgram(db)
	p.Commands = append(p.Commands, probe.command())

	got := testkit.Run(t, p, "", "probe")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("probe = %d with %q, want %d", got.Code, got.Stderr, gonsole.ExitDone)
	}
	if !probe.held || probe.before != nil {
		t.Errorf("Held() before the release = %t, %v, want the records table found", probe.held, probe.before)
	}
	wantUnder(t, probe.after, "check the records")
	db.noSessionLeft()
}

func TestTheStoresPassAMissingDatabaseSettingThroughUnchanged(t *testing.T) {
	t.Parallel()

	_, want := gonsole.Call{}.DatabaseURL()
	if want == nil {
		t.Fatal("gonsole.Call{}.DatabaseURL() error = nil, want the failure of a call with no database setting")
	}

	_, release, err := postgres.Stores(t.Context(), gonsole.Call{})

	if fmt.Sprint(err) != want.Error() || release != nil {
		t.Errorf("Stores() = a release %t, %v, want no release and %v as it is", release != nil, err, want)
	}
}

func TestTheStoresNameAnUnreadableAddressWithoutItsPassword(t *testing.T) {
	t.Parallel()

	db := testDB{t: t, address: "postgres://u:" + newSecret + "/tail@localhost:5434/app"}

	got := testkit.Run(t, moduleProgram(db), "", "account:list")

	if !strings.HasPrefix(got.Stderr, "myapp: postgres: open the database: ") || strings.Contains(got.Stderr, newSecret) {
		t.Errorf("account:list = %d with %q, want the database named and no part of the password", got.Code, got.Stderr)
	}
}

func TestTheStoresNameAnAddressThePoolRefuses(t *testing.T) {
	t.Parallel()

	db := migrated(t).withQuery("pool_max_conns", "0")

	got := testkit.Run(t, moduleProgram(db), "", "account:list")

	if got.Code != gonsole.ExitFailed || !strings.HasPrefix(got.Stderr, "myapp: postgres: open the database: ") {
		t.Errorf("account:list = %d with %q, want %d naming the database", got.Code, got.Stderr, gonsole.ExitFailed)
	}
}
