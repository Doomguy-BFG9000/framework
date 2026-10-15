// SPDX-License-Identifier: Apache-2.0

package dbkit_test

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gopherium/framework/dbkit"
)

// intxTimeout is the deadline of the InTx tests whose driver holds a call until the context ends.
const intxTimeout = time.Second

// errBody is what a failing InTx body returns.
var errBody = errors.New("body failed")

// wantCalls fails t unless the fake driver received exactly ops, in order.
func wantCalls(t *testing.T, fake *fakeDriver, ops ...string) {
	t.Helper()
	if got := fake.log(); !slices.Equal(got, ops) {
		t.Errorf("driver calls = %v, want %v", got, ops)
	}
}

func TestInTxCommitsWhenBodySucceeds(t *testing.T) {
	t.Parallel()
	db, fake := shareHandle(t)

	err := dbkit.InTx(t.Context(), db, func(*sql.Tx) error { return nil })

	if err != nil {
		t.Errorf("InTx() error = %v, want nil", err)
	}
	wantCalls(t, fake, "begin", "commit")
}

func TestInTxRollsBackWhenBodyReturnsAnError(t *testing.T) {
	t.Parallel()
	db, fake := shareHandle(t)

	err := dbkit.InTx(t.Context(), db, func(*sql.Tx) error { return errBody })

	if !errors.Is(err, errBody) {
		t.Errorf("InTx() error = %v, want %v", err, errBody)
	}
	wantCalls(t, fake, "begin", "rollback")
}

func TestInTxRollsBackBeforeRethrowingAPanic(t *testing.T) {
	t.Parallel()
	db, fake := shareHandle(t)
	want := "body panicked"

	defer func() {
		if got := recover(); got != want {
			t.Errorf("panic = %v, want %q", got, want)
		}
		wantCalls(t, fake, "begin", "rollback")
	}()
	_ = dbkit.InTx(t.Context(), db, func(*sql.Tx) error { panic(want) })
}

func TestInTxRollsBackWhenBodyNeverReturns(t *testing.T) {
	t.Parallel()
	db, fake := shareHandle(t)
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = dbkit.InTx(t.Context(), db, func(*sql.Tx) error {
			runtime.Goexit()
			return nil
		})
	}()
	<-done

	wantCalls(t, fake, "begin", "rollback")
}

func TestInTxAnswersTheEndOfItsContext(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		db, fake := shareHandle(t)
		ctx, cancel := context.WithCancel(t.Context())

		err := dbkit.InTx(ctx, db, func(*sql.Tx) error {
			cancel()
			synctest.Wait()
			return nil
		})

		if !errors.Is(err, context.Canceled) {
			t.Errorf("InTx() error = %v, want it to match context.Canceled", err)
		}
		wantCalls(t, fake, "begin", "rollback")
	})
}

func TestInTxAnswersABeginFailure(t *testing.T) {
	t.Parallel()
	db, fake := shareHandle(t)
	fake.fail("")
	ran := false

	err := dbkit.InTx(t.Context(), db, func(*sql.Tx) error {
		ran = true
		return nil
	})

	if !errors.Is(err, fakeBroken) {
		t.Errorf("InTx() error = %v, want %v", err, fakeBroken)
	}
	if ran {
		t.Error("the body ran, want it never to run once the begin failed")
	}
}

func TestInTxAnswersACommitFailure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		db, fake := shareHandle(t)
		fake.holdCommit()
		ctx, cancel := context.WithTimeout(t.Context(), intxTimeout)
		defer cancel()

		err := dbkit.InTx(ctx, db, func(*sql.Tx) error { return nil })

		wantFailedCommit(t, err)
		wantCalls(t, fake, "begin", "commit")
	})
}

// wantFailedCommit fails t unless err is the driver's commit error marked with the deadline, never sql.ErrTxDone.
func wantFailedCommit(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, fakeInterrupted) {
		t.Errorf("InTx() error = %v, want the driver error %v", err, fakeInterrupted)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("InTx() error = %v, want it to match context.DeadlineExceeded", err)
	}
	if errors.Is(err, sql.ErrTxDone) {
		t.Errorf("InTx() error = %v, want no %v joined", err, sql.ErrTxDone)
	}
}

func TestInTxJoinsAFailedRollbackToTheBodysError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		db, fake := shareHandle(t)
		fake.holdRollback()
		ctx, cancel := context.WithTimeout(t.Context(), intxTimeout)
		defer cancel()

		err := dbkit.InTx(ctx, db, func(*sql.Tx) error { return errBody })

		if !errors.Is(err, errBody) {
			t.Errorf("InTx() error = %v, want the body's %v", err, errBody)
		}
		if !errors.Is(err, fakeInterrupted) {
			t.Errorf("InTx() error = %v, want the rollback's %v joined", err, fakeInterrupted)
		}
		wantCalls(t, fake, "begin", "rollback")
	})
}
