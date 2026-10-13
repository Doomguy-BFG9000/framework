// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/testkit"
)

// stored is one command record as the records fake holds it.
type stored struct {
	actor, account, command string
	args                    []string
	flags                   map[string]string
}

// guarded is a config over the in-memory stores naming manage_users, recording within a second.
func guarded() auth.Config {
	return auth.Config{
		Roles:         func(context.Context, gonsole.Call) (auth.Roles, error) { return vocabulary, nil },
		Capability:    "manage_users",
		RecordTimeout: time.Second,
		Stores:        memoryStores,
	}
}

// recording returns a program over the world at address whose applied guarded commands cfg records.
func recording(
	address string, cfg auth.Config, settings map[string]string, commands ...gonsole.Command,
) gonsole.Program {
	values := map[string]string{"MYAPP_DATABASE_URL": address}
	for key, value := range settings {
		values[key] = value
	}
	p := program(address, commands...)
	p.Env = gonsole.Env{Prefix: "MYAPP_", Getenv: testkit.Getenv(values)}
	p.Record = auth.Record(cfg)
	return p
}

// recorded returns the address of a seeded world the records step also ran on.
func recorded(t *testing.T) string {
	t.Helper()
	address := seeded(t)
	if err := recordsStep.Run(t.Context(), address); err != nil {
		t.Fatalf("applying the records schema: %v", err)
	}
	return address
}

// records returns every record the world at address holds, oldest first.
func records(t *testing.T, address string) []stored {
	t.Helper()
	var held []stored
	for _, entry := range worldAt(t, address).records.all() {
		held = append(held, stored{
			actor: entry.Actor, account: accountID(entry), command: entry.Command, args: entry.Args, flags: entry.Flags,
		})
	}
	return held
}

// accountID returns the id of the account entry names, empty for none.
func accountID(entry auth.Entry) string {
	if entry.AccountID == nil {
		return ""
	}
	return *entry.AccountID
}

// command returns a guarded command named name that writes nothing and prints done.
func command(name string) gonsole.Command {
	return gonsole.Command{
		Name: name, Summary: "do nothing", Capability: "manage_users",
		Run: func(_ context.Context, call gonsole.Call) error {
			_, err := fmt.Fprintln(call.Stdout, "done")
			return err
		},
	}
}

func TestRecordStoresTheActorTheCommandAndItsArguments(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	p := recording(address, guarded(), nil, auth.Commands(guarded())...)

	got := testkit.Run(t, p, "", "account:role", "Editor@Example.com", "author", "-yes", "-as", " Admin@Example.com ")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("code %d, want 0, stderr %q", got.Code, got.Stderr)
	}
	admin := account(t, storeAt(t, address), "admin@example.com")
	want := []stored{{
		actor: "admin@example.com", account: admin.ID.String(), command: "account:role",
		args: []string{"Editor@Example.com", "author"}, flags: map[string]string{},
	}}
	if held := records(t, address); !reflect.DeepEqual(held, want) {
		t.Errorf("records = %#v, want %#v", held, want)
	}
}

func TestRecordStoresATimeOrderedID(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	p := recording(address, guarded(), nil, command("report:purge"))
	testkit.Run(t, p, "", "report:purge", "-as", "admin@example.com")
	var version uuid.Version

	held := worldAt(t, address).records.all()
	if len(held) == 1 {
		version = versionOf(held[0].ID)
	}

	if version != 7 {
		t.Errorf("records %+v, id version %d, want one record under a UUIDv7", held, version)
	}
}

// versionOf returns the version of id when it is a UUID in its canonical text, zero otherwise.
func versionOf(id string) uuid.Version {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return 0
	}
	return parsed.Version()
}

func TestRecordStoresNothingForADryRun(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	p := authorizing(address, checked(), nil, auth.Commands(checked())...)

	got := testkit.Run(t, p, "", "account:role", "editor@example.com", "author", "-as", "admin@example.com")

	if held := records(t, address); got.Code != gonsole.ExitDone || len(held) != 0 {
		t.Errorf("code %d, records %v, want 0 and none, stderr %q", got.Code, held, got.Stderr)
	}
}

func TestRecordStoresAnEmptyListForACommandWithoutArguments(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	p := recording(address, guarded(), nil, auth.Commands(guarded())...)

	got := testkit.Run(t, p, "", "account:grant-role", "-role", "editor", "-yes", "-as", "admin@example.com")

	held := records(t, address)
	if got.Code != gonsole.ExitDone || len(held) != 1 {
		t.Fatalf("code %d, records %v, want 0 and one record, stderr %q", got.Code, held, got.Stderr)
	}
	typed, want := []any{held[0].args, held[0].flags}, []any{[]string{}, map[string]string{"role": "editor"}}
	if !reflect.DeepEqual(typed, want) {
		t.Errorf("args and flags %#v, want an empty list and the role flag", typed)
	}
}

func TestRecordStoresNoAccountForAnAddressNoAccountHolds(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	p := recording(address, guarded(), nil, command("report:purge"))

	got := testkit.Run(t, p, "", "report:purge", "-as", "nobody@example.com")

	held := records(t, address)
	want := []stored{{actor: "nobody@example.com", command: "report:purge", args: []string{}, flags: map[string]string{}}}
	if got.Code != gonsole.ExitDone || !reflect.DeepEqual(held, want) {
		t.Errorf("code %d, records %#v, want 0 and one record with no account", got.Code, held)
	}
}

func TestRecordNamesAMalformedTimeout(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	p := recording(address, guarded(), map[string]string{"MYAPP_COMMAND_RECORD_TIMEOUT": "soon"}, command("report:purge"))

	got := testkit.Run(t, p, "", "report:purge", "-as", "admin@example.com")

	want := `myapp: MYAPP_COMMAND_RECORD_TIMEOUT: must be a duration like 30s, got "soon"` + "\n"
	if got.Code != gonsole.ExitFailed || got.Stderr != want {
		t.Errorf("code %d, stderr %q, want 1 and %q", got.Code, got.Stderr, want)
	}
	if held := records(t, address); len(held) != 0 {
		t.Errorf("records = %v, want none", held)
	}
}

func TestRecordRefusesATimeoutThatIsNotAboveZero(t *testing.T) {
	t.Parallel()

	address := recorded(t)
	cfg := guarded()
	cfg.RecordTimeout = 0
	p := recording(address, cfg, nil, command("report:purge"))

	got := testkit.Run(t, p, "", "report:purge", "-as", "admin@example.com")

	want := "myapp: gonsole/auth: the record timeout must stand above zero, got 0s\n"
	if got.Code != gonsole.ExitFailed || got.Stderr != want {
		t.Errorf("code %d, stderr %q, want 1 and %q", got.Code, got.Stderr, want)
	}
}
