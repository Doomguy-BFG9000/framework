// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit"
	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"
	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/testkit"
)

func TestRecordWaitsNoLongerThanTheBusyTimeout(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.BusyTimeout = 2 * time.Second
	opts.MaxConns = 3
	db := sqlitetest.Open(t, opts)
	applied(t, db)
	writer, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("beginning the writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Rollback() })
	if _, err := writer.ExecContext(t.Context(), "CREATE TABLE core_items (id TEXT NOT NULL PRIMARY KEY)"); err != nil {
		t.Fatalf("writing inside the writer: %v", err)
	}
	cfg := testConfig()
	cfg.RecordTimeout = 20 * time.Millisecond
	call := gonsole.Call{
		Env: gonsole.Env{Prefix: "MYAPP_", Getenv: testkit.Getenv(nil)}, Actor: "admin@example.com",
		Args: []string{"author@example.com"},
	}.WithDB(db)

	start := time.Now()
	err = auth.Record(cfg)(context.Background(), call, "account:disable")
	waited := time.Since(start)

	if err == nil || !strings.HasPrefix(err.Error(), "record account:disable: sqlite: insert a record: ") ||
		!errors.Is(err, context.DeadlineExceeded) || errors.Is(err, dbkit.ErrBusy) {
		t.Errorf("Record() behind a held write = %v, want the record's deadline under the insert", err)
	}
	if waited < cfg.RecordTimeout || waited >= 2*opts.BusyTimeout {
		t.Errorf("Record() returned after %v, want at least the record timeout %v and under twice the busy timeout %v",
			waited, cfg.RecordTimeout, opts.BusyTimeout)
	}
}
