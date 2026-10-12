// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"testing"

	authkitsqlite "github.com/gopherium/gouncer/authkit/sqlite"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/framework/gonsole/auth/recordstest"
	"github.com/gopherium/framework/gonsole/auth/sqlite"
)

// recordsFixture returns the contract fixture of a record store over a fresh copy holding the account tables only.
func recordsFixture(t *testing.T) recordstest.Fixture {
	db := accountsOnly(t)
	accounts := authkitsqlite.NewUserStore(db)
	return recordstest.Fixture{
		Records: sqlite.NewRecordStore(db),
		Migrate: func(ctx context.Context) error {
			return sqlite.RecordMigration(testMigrate).Apply(ctx, "", db)
		},
		Account: func(ctx context.Context, email string) (string, error) {
			user, err := gouncer.NewUser(email, "recorded account", "correct horse battery")
			if err != nil {
				return "", err
			}
			return user.ID.String(), accounts.CreateUser(ctx, user)
		},
		Plant: sqlite.PlantOn(db),
	}
}

func TestSQLiteRecordsKeepTheContract(t *testing.T) {
	t.Parallel()

	recordstest.Run(t, recordsFixture)
}
