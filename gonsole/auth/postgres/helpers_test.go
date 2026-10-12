// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit/postgres/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopherium/framework/dbkit/postgres/pgtest"
	"github.com/gopherium/framework/gonsole/auth/postgres"
)

const (
	// serverVariable names the environment variable that holds the address of the test server.
	serverVariable = "DBKIT_TEST_POSTGRES_URL"
	// demoPassword is the password of every account the tests create.
	demoPassword = "correct horse battery"
	// newSecret is a part of an address password that no message may hold.
	newSecret = "secretword"
	// tableLookup answers whether the table its one argument names exists.
	tableLookup = "SELECT to_regclass($1) IS NOT NULL"
	// lockTaking takes goose's migration lock.
	lockTaking = "SELECT pg_advisory_lock($1)"
	// lockRelease lets goose's migration lock go.
	lockRelease = "SELECT pg_advisory_unlock($1)"
	// sessionLookup answers whether a session other than this one and the holder's has a column like a pattern.
	sessionLookup = "SELECT EXISTS (SELECT FROM pg_stat_activity WHERE datname = current_database()" +
		" AND pid NOT IN (pg_backend_pid(), $1) AND %s LIKE $2)"
	// noOtherSession answers whether no client session but this one is on the database.
	noOtherSession = "SELECT NOT EXISTS (SELECT FROM pg_stat_activity WHERE datname = current_database()" +
		" AND backend_type = 'client backend' AND pid <> pg_backend_pid())"
	// sessionBound is how long noSessionLeft waits for the sessions of a run to end.
	sessionBound = 5 * time.Second
)

// server is the address of the test server.
var server string

// TestMain runs the tests once serverVariable holds an address pgtest takes.
func TestMain(m *testing.M) {
	server = os.Getenv(serverVariable)
	if err := pgtest.CheckAddress(server); err != nil {
		_, _ = fmt.Fprintf(os.Stderr,
			"gonsole/auth/postgres: the tests need %s, the address of a PostgreSQL server pgtest takes: %v\n",
			serverVariable, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// sessionState is the column of pg_stat_activity a waiting session shows and the pattern it matches.
type sessionState struct {
	// column is the column of pg_stat_activity.
	column string
	// pattern is the LIKE pattern the column matches.
	pattern string
}

var (
	// waitingOnLock is a session waiting on a lock.
	waitingOnLock = sessionState{column: "wait_event_type", pattern: "Lock"}
	// waitingOnAdvisory is a session asking for an advisory lock.
	waitingOnAdvisory = sessionState{column: "query", pattern: "%advisory%"}
)

// testDB is one fresh database of one test.
type testDB struct {
	// t is the test the database belongs to.
	t *testing.T
	// address is the address of the database.
	address string
}

// empty returns a fresh database holding no schema.
func empty(t *testing.T) testDB {
	t.Helper()
	return testDB{t: t, address: pgtest.New(t, server, pgtestdb.NoopMigrator{})}
}

// migrated returns a fresh database holding the account schema and no records table.
func migrated(t *testing.T) testDB {
	t.Helper()
	return testDB{t: t, address: pgtest.New(t, server, testdb.Migrator())}
}

// recorded returns a fresh database holding the account schema and the records table.
func recorded(t *testing.T) testDB {
	t.Helper()
	db := migrated(t)
	db.apply(postgres.RecordMigration())
	return db
}

// withQuery returns db at an address whose query sets key to value.
func (db testDB) withQuery(key, value string) testDB {
	db.t.Helper()
	address, err := url.Parse(db.address)
	if err != nil {
		db.t.Fatalf("parsing the test database address: %v", err)
	}
	query := address.Query()
	query.Set(key, value)
	address.RawQuery = query.Encode()
	return testDB{t: db.t, address: address.String()}
}

// dial returns a new connection to db, which the caller closes.
func (db testDB) dial() *pgx.Conn {
	db.t.Helper()
	conn, err := pgx.Connect(db.t.Context(), db.address)
	if err != nil {
		db.t.Fatalf("connecting to the test database: %v", err)
	}
	return conn
}

// connect returns a connection to db, closed when the test ends.
func (db testDB) connect() *pgx.Conn {
	db.t.Helper()
	conn := db.dial()
	db.t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// run executes each statement on db, failing the test at the first that fails.
func (db testDB) run(statements ...string) {
	db.t.Helper()
	conn := db.dial()
	defer func() { _ = conn.Close(context.Background()) }()
	for _, statement := range statements {
		if _, err := conn.Exec(db.t.Context(), statement); err != nil {
			db.t.Fatalf("%s: %v", statement, err)
		}
	}
}

// exists reports whether db holds the table named name.
func (db testDB) exists(name string) bool {
	db.t.Helper()
	return db.found(tableLookup, name)
}

// found reports whether lookup answers true for values on db.
func (db testDB) found(lookup string, values ...any) bool {
	db.t.Helper()
	conn := db.dial()
	defer func() { _ = conn.Close(context.Background()) }()
	var held bool
	if err := conn.QueryRow(db.t.Context(), lookup, values...).Scan(&held); err != nil {
		db.t.Fatalf("%s: %v", lookup, err)
	}
	return held
}

// pool returns a pool on db, closed when the test ends.
func (db testDB) pool() *pgxpool.Pool {
	db.t.Helper()
	pool, err := pgxpool.New(db.t.Context(), db.address)
	if err != nil {
		db.t.Fatalf("opening a pool on the test database: %v", err)
	}
	db.t.Cleanup(pool.Close)
	return pool
}

// holdMigrationLock returns a connection holding goose's migration lock on db.
func (db testDB) holdMigrationLock() *pgx.Conn {
	db.t.Helper()
	conn := db.connect()
	if _, err := conn.Exec(db.t.Context(), lockTaking, lock.DefaultLockID); err != nil {
		db.t.Fatalf("holding the migration lock: %v", err)
	}
	return conn
}

// noSessionLeft fails the test unless every client session but its own leaves db within sessionBound.
func (db testDB) noSessionLeft() {
	db.t.Helper()
	deadline := time.Now().Add(sessionBound)
	for !db.found(noOtherSession) {
		if time.Now().After(deadline) {
			db.t.Fatalf("a session stayed on the test database %v after the run, want every pool closed", sessionBound)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// awaitSession returns once a session other than holder is in state on db, failing if ran ends first.
func (db testDB) awaitSession(holder *pgx.Conn, ran <-chan error, state sessionState) {
	db.t.Helper()
	lookup := fmt.Sprintf(sessionLookup, state.column)
	for !db.found(lookup, int32(holder.PgConn().PID()), state.pattern) {
		select {
		case err := <-ran:
			db.t.Fatalf("Run() = %v before a session had %s like %s", err, state.column, state.pattern)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
