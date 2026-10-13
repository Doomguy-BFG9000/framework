// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/recordstest"
)

// memoryFixture returns the records fake of a fresh world and the hooks recordstest drives it with.
func memoryFixture(t *testing.T) recordstest.Fixture {
	t.Helper()
	address := migrated(t)
	held := worldAt(t, address)
	return recordstest.Fixture{
		Records: held.records,
		Migrate: func(ctx context.Context) error { return recordsStep.Run(ctx, address) },
		Account: func(ctx context.Context, email string) (string, error) {
			created := auth.Account{Email: email, Name: "Maria Perez", Password: demoPassword, Role: "admin"}
			if err := auth.EnsureAccounts(ctx, held.accounts, []auth.Account{created}, io.Discard); err != nil {
				return "", err
			}
			user, err := held.accounts.UserByEmail(ctx, email)
			return user.ID.String(), err
		},
		Plant: held.records.plant,
	}
}

func TestTheInMemoryRecordsKeepTheContract(t *testing.T) {
	t.Parallel()

	recordstest.Run(t, memoryFixture)
}

func TestAnEntryKeepsTheDocumentAccountRecordsAnswers(t *testing.T) {
	t.Parallel()

	accountID := "0193f1a2-7b3c-7d4e-8f5a-6b7c8d9e0f1a"
	entry := auth.Entry{
		ID: "0193f1a2-7b3c-7d4e-8f5a-000000000001", AppliedAt: time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC),
		Actor: "admin@example.com", AccountID: &accountID, Command: "account:disable",
		Args: []string{"author@example.com"}, Flags: map[string]string{},
	}

	document, err := json.Marshal(entry)

	want := `{"applied_at":"2026-10-09T08:30:00Z","actor":"admin@example.com",` +
		`"account_id":"0193f1a2-7b3c-7d4e-8f5a-6b7c8d9e0f1a","command":"account:disable",` +
		`"args":["author@example.com"],"flags":{}}`
	if err != nil || string(document) != want {
		t.Errorf("json.Marshal() = %s, %v, want %s", document, err, want)
	}
}
