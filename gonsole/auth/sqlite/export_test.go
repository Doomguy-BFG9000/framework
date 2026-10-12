// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/gopherium/framework/gonsole/auth"
)

const (
	// HeldRecords is the exact text of the statement Held runs.
	HeldRecords = heldRecords
	// StoreRecord is the exact text of the statement Insert runs.
	StoreRecord = storeRecord
	// LatestRecords is the exact text of the statement Latest runs.
	LatestRecords = latestRecords
)

// PlantOn returns the hook that stores an entry in db as Insert does, but as applied at appliedAt.
func PlantOn(db *sql.DB) func(ctx context.Context, entry auth.Entry, appliedAt time.Time) error {
	return recordStore{db: db}.insert
}
