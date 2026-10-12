// SPDX-License-Identifier: Apache-2.0

package pgtest_test

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit/postgres/pgtest"
)

// sweptWait is how long a later caller must stay blocked behind a sweep to show that it waits.
const sweptWait = 300 * time.Millisecond

// sweptStale returns the name of a stale instance of the template that scope names, created and aged for the test.
func sweptStale(t *testing.T, scope string) string {
	t.Helper()
	name := "testdb_tpl_" + scope + "_inst_00000001"
	sweepCreate(t, name)
	sweepAwaitOld(t, name)
	return name
}

func TestNewSweptSweepsEachAddressOnceThenHandsOutADatabase(t *testing.T) {
	t.Parallel()

	_, names, scope := sweepInstances(t, 1)
	migrator := newMigrator()
	other := serverAddressWith(t, "application_name", serverName("swept-"))

	first := pgtest.NewSweptWithin(t, serverAddress(), sweepAnyAge, migrator, scope)
	serverDropTemplateAtEnd(t, serverDatabase(t, first))
	sweptFirst := !sweepExists(t, names[0])
	stale := sweptStale(t, scope)
	second := pgtest.NewSweptWithin(t, serverAddress(), sweepAnyAge, migrator, scope)
	keptBySameAddress := sweepExists(t, stale)
	third := pgtest.NewSweptWithin(t, other, sweepAnyAge, migrator, scope)

	if !sweptFirst || !keptBySameAddress {
		t.Errorf("the stale instances were swept by the first call %t and kept by the second %t, want true and true",
			sweptFirst, keptBySameAddress)
	}
	if sweepExists(t, stale) {
		t.Error("the stale instance outlived the call on another address, want that call to sweep it")
	}
	for _, address := range []string{first, second, third} {
		if got := newRows(t, address); got != 1 {
			t.Errorf("a database NewSwept returned holds %d rows, want the migrated row", got)
		}
	}
}

func TestNewSweptMakesALaterCallerWaitForTheSweep(t *testing.T) {
	t.Parallel()

	_, names, scope := sweepInstances(t, 1)
	migrator := newMigrator()
	heldLock := sweepLock(t, names[0])
	first, second := make(chan string, 1), make(chan string, 1)
	go func() { first <- pgtest.NewSweptWithin(t, serverAddress(), sweepAnyAge, migrator, scope) }()
	sweepAwaitDrops(t, names[0], 1, nil)
	go func() { second <- pgtest.NewSweptWithin(t, serverAddress(), sweepAnyAge, migrator, scope) }()

	var address string
	select {
	case address = <-second:
		t.Error("the second NewSwept() returned while the first one's sweep was still dropping, want it to wait")
	case <-time.After(sweptWait):
	}
	if err := heldLock.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}
	serverDropTemplateAtEnd(t, serverDatabase(t, <-first))
	if address == "" {
		address = <-second
	}
	if newRows(t, address) != 1 || sweepExists(t, names[0]) {
		t.Errorf("after the sweep the second database holds %d rows and the stale one exists %t, want 1 and false",
			newRows(t, address), sweepExists(t, names[0]))
	}
}

func TestNewSweptLogsAFailedSweepAndHandsOutADatabase(t *testing.T) {
	t.Parallel()

	role, password := serverName("dbkit_swept_"), "swept-role-password"
	serverExec(t, "CREATE ROLE %I LOGIN CREATEDB PASSWORD %L", role, password)
	t.Cleanup(func() { serverExec(t, "DROP ROLE %I", role) })
	migrator := newMigrator()
	serverDropTemplateAtEnd(t, serverDatabase(t, pgtest.New(t, serverAddress(), migrator)))
	serverExec(t, "GRANT pgtdbuser TO %I", role)
	server, err := url.Parse(serverAddress())
	if err != nil {
		t.Fatal("the test server address cannot be parsed")
	}
	server.User = url.UserPassword(role, password)
	recorder := &newFatal{TB: t}
	var address string
	done := make(chan struct{})
	go func() {
		defer close(done)
		address = pgtest.NewSweptWithin(recorder, server.String(), sweepCentury, migrator, serverName("never-"))
	}()
	<-done

	logged := strings.Join(recorder.output, "\n")
	want := "dbkit: the sweep of stale test databases failed, the tests go on: dbkit: Sweep needs a role that may run " +
		"pg_stat_file"
	if recorder.message != "" || address == "" {
		t.Errorf("NewSwept() failed the test with %q and returned %q, want no failure and a database",
			recorder.message, address)
	}
	if !strings.Contains(logged, want) {
		t.Errorf("NewSwept() logged %q, want %q", logged, want)
	}
}

// sweptRun returns the recorder that run printed through once it ended.
func sweptRun(t *testing.T, run func(tb testing.TB)) *newFatal {
	t.Helper()
	recorder := &newFatal{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(recorder)
	}()
	<-done
	return recorder
}

func TestNewSweptRefusesAnAgeAtOrBelowZero(t *testing.T) {
	t.Parallel()

	refused := sweepUnreachable + "&user=moved_user"
	for age, want := range map[time.Duration]string{
		0:                "dbkit: NewSwept needs olderThan above zero, got 0s",
		-time.Nanosecond: "dbkit: NewSwept needs olderThan above zero, got -1ns",
	} {
		recorder := sweptRun(t, func(tb testing.TB) { pgtest.NewSwept(tb, refused, age, newMigrator()) })

		if recorder.message != want {
			t.Errorf("NewSwept() with the age %v failed the test with %q, want %q", age, recorder.message, want)
		}
	}
}

func TestNewSweptRefusesAnAddressBeforeItSweeps(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		sweepUnreachable + "&user=moved_user",
		sweepUnreachable + "&pool_max_conn_idle_time=1m",
		"host=127.0.0.1 port=1 user=u password=p dbname=x sslmode=disable",
	} {
		recorder := sweptRun(t, func(tb testing.TB) {
			pgtest.NewSweptWithin(tb, address, sweepAnyAge, newMigrator(), serverName("refused-"))
		})

		want := fmt.Sprint(pgtest.CheckAddress(address))
		if recorder.message != want || !slices.Equal(recorder.output, []string{want}) {
			t.Errorf("NewSwept() printed %q, want only the refusal %q of CheckAddress", recorder.output, want)
		}
	}
}
