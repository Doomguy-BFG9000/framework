// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// migrationContents returns the contents of each migration file in folder, keyed by its name.
func migrationContents(t *testing.T, folder string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("reading %s: %v", folder, err)
	}
	contents := map[string]string{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		contents[entry.Name()] = string(data)
	}
	return contents
}

func TestRecordsMigrationsMatchGonsoleAuthByteForByte(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat(filepath.Join("..", "go.mod")); errors.Is(err, os.ErrNotExist) {
		t.Skip("gonsole/auth is not beside this module")
	}

	module := migrationContents(t, "migrations")
	core := migrationContents(t, filepath.Join("..", "migrations"))

	if !maps.Equal(module, core) {
		t.Errorf("the records migrations are %v here and %v in gonsole/auth, want the same files byte for byte",
			slices.Sorted(maps.Keys(module)), slices.Sorted(maps.Keys(core)))
	}
}
