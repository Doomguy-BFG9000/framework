// SPDX-License-Identifier: Apache-2.0

package postgres

import "testing"

func TestMustSubPanicsOnAnInvalidPath(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("mustSub() returned, want a panic on an invalid path")
		}
	}()

	mustSub(migrationFiles, "..")
}
