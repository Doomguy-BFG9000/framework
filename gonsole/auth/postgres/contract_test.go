// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"io"
	"maps"
	"testing"
	"time"

	authkitpostgres "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/auth/postgres"
	"github.com/gopherium/framework/gonsole/auth/recordstest"
)

// plantAt inserts one record as applied at a given time, finding the account the acting address names.
const plantAt = `INSERT INTO gonsole.records (id, applied_at, actor, account_id, command, args, flags)
VALUES ($1::uuid, $2, $3, (SELECT id FROM auth.users WHERE email = $3), $4, $5::jsonb, $6::jsonb)`

var _ auth.Accounts = (*authkitpostgres.UserStore)(nil)

// recordsFixture returns the contract fixture of a record store over a fresh database holding the accounts only.
func recordsFixture(t *testing.T) recordstest.Fixture {
	t.Helper()
	db := migrated(t)
	pool := db.pool()
	accounts := authkitpostgres.NewUserStore(pool)
	return recordstest.Fixture{
		Records: postgres.NewRecordStore(pool),
		Migrate: func(ctx context.Context) error { return postgres.RecordMigration().Run(ctx, db.address) },
		Account: func(ctx context.Context, email string) (string, error) {
			held := auth.Account{Email: email, Name: "Maria Perez", Password: demoPassword, Role: "admin"}
			if err := auth.EnsureAccounts(ctx, accounts, []auth.Account{held}, io.Discard); err != nil {
				return "", err
			}
			user, err := accounts.UserByEmail(ctx, email)
			return user.ID.String(), err
		},
		Plant: func(ctx context.Context, entry auth.Entry, appliedAt time.Time) error {
			flags := map[string]string{}
			maps.Copy(flags, entry.Flags)
			_, err := pool.Exec(ctx, plantAt, entry.ID, appliedAt, entry.Actor, entry.Command,
				append([]string{}, entry.Args...), flags)
			return err
		},
	}
}

func TestPostgresRecordsKeepTheContract(t *testing.T) {
	t.Parallel()

	recordstest.Run(t, recordsFixture)
}
