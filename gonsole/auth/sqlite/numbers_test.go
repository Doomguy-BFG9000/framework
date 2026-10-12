// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// migrationNumbers returns the version numbers of the migration files in folder.
func migrationNumbers(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("reading %s: %v", folder, err)
	}
	var numbers []string
	for _, entry := range entries {
		number, _, found := strings.Cut(entry.Name(), "_")
		if found && strings.HasSuffix(entry.Name(), ".sql") {
			numbers = append(numbers, number)
		}
	}
	return numbers
}

func TestRecordsMigrationNumbersMatchGonsoleAuthPostgres(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat(filepath.Join("..", "postgres", "go.mod")); errors.Is(err, os.ErrNotExist) {
		t.Skip("gonsole/auth/postgres is not beside this module")
	}

	sqliteNumbers := migrationNumbers(t, "migrations")
	postgresNumbers := migrationNumbers(t, filepath.Join("..", "postgres", "migrations"))

	if !slices.Equal(sqliteNumbers, postgresNumbers) {
		t.Errorf("records migration numbers are %v on SQLite and %v on PostgreSQL, want the same",
			sqliteNumbers, postgresNumbers)
	}
}
