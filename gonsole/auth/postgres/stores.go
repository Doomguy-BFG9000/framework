// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"

	authkitpostgres "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
)

// Stores returns the stores of call on a pool of its database setting, and the release that closes the pool.
func Stores(ctx context.Context, call gonsole.Call) (auth.Stores, func(context.Context) error, error) {
	address, err := call.DatabaseURL()
	if err != nil {
		return auth.Stores{}, nil, err
	}
	pool, err := pgxpool.New(ctx, address)
	if err != nil {
		return auth.Stores{}, nil, errUnreadable
	}
	release := func(context.Context) error {
		pool.Close()
		return nil
	}
	return auth.Stores{Accounts: authkitpostgres.NewUserStore(pool), Records: NewRecordStore(pool)}, release, nil
}
