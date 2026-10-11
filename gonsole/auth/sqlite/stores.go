// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"

	authkitsqlite "github.com/gopherium/gouncer/authkit/sqlite"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
)

// Stores returns the account and record stores of call on the run's one handle, which the run owns and closes.
func Stores(ctx context.Context, call gonsole.Call) (auth.Stores, func(context.Context) error, error) {
	db, err := call.DB(ctx)
	if err != nil {
		return auth.Stores{}, nil, err
	}
	return auth.Stores{Accounts: authkitsqlite.NewUserStore(db), Records: NewRecordStore(db)}, nil, nil
}
