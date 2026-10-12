// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"regexp"
	"testing"

	"github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/gonsole/testkit"
)

var (
	// stamps matches the times a line prints.
	stamps = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	// ids matches the UUIDs a line prints.
	ids = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

// answer is what one line answered, its times and ids cut.
type answer struct {
	// code is the exit code.
	code int
	// stdout is what the line printed.
	stdout string
	// stderr is what the line printed as failure.
	stderr string
}

// cut returns the answer of result with each time and id replaced by a placeholder.
func cut(result testkit.Result) answer {
	clean := func(text string) string { return ids.ReplaceAllString(stamps.ReplaceAllString(text, "<time>"), "<id>") }
	return answer{code: result.Code, stdout: clean(result.Stdout), stderr: clean(result.Stderr)}
}

func TestEveryLineAnswersAsGonsoleAuthDoesWithNoStoresBuilder(t *testing.T) {
	t.Parallel()

	core := program(empty(t).address, testConfig(nil), auth.Migration(), auth.RecordMigration())
	module := moduleProgram(empty(t))
	asAdmin := []string{"-yes", "-as", "admin@example.com"}

	for _, line := range []struct {
		stdin string
		args  []string
	}{
		{"", []string{"migrate"}},
		{demoPassword + "\n", []string{"account:create-admin", "-email", "admin@example.com", "-name", "Maria Perez",
			"-role", "admin"}},
		{demoPassword + "\n", []string{"account:create-admin", "-email", "author@example.com", "-name", "Maria Perez",
			"-role", "author"}},
		{"", append([]string{"account:grant-role", "-role", "editor"}, asAdmin...)},
		{"", []string{"account:list"}},
		{"", append([]string{"account:role", "author@example.com", "editor"}, asAdmin...)},
		{"", append([]string{"account:disable", "author@example.com"}, asAdmin...)},
		{"", []string{"account:role", "admin@example.com", "editor", "-yes", "-as", "author@example.com"}},
		{"", []string{"account:records"}},
		{"", []string{"account:records", "-json"}},
	} {
		want := cut(testkit.Run(t, core, line.stdin, line.args...))

		got := cut(testkit.Run(t, module, line.stdin, line.args...))

		if got != want {
			t.Errorf("%v answers %+v through the module, want %+v as gonsole/auth answers", line.args, got, want)
		}
	}
}
