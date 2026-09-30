package config

import (
	"strings"
	"testing"
)

// A path check has to say what it runs and name a file inside the repository
// listing what it vouches for; anything else is refused naming the entry.
func TestAPathCheckNamesItsCommandAndAListInsideTheRepository(t *testing.T) {
	t.Parallel()

	if problems := (PathCheck{Command: "make adoption", Paths: "scripts/walk-adoption.paths"}).problems(0); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
	for _, test := range []struct {
		check PathCheck
		want  string
	}{
		{check: PathCheck{Paths: "scripts/walk-adoption.paths"}, want: "command cannot be empty"},
		{check: PathCheck{Command: "make adoption"}, want: "paths must name the file"},
		{check: PathCheck{Command: "make adoption", Paths: "/etc/paths"}, want: "must be relative"},
		{check: PathCheck{Command: "make adoption", Paths: "../elsewhere.paths"}, want: "inside the repository"},
		{check: PathCheck{Command: "make adoption", Paths: "."}, want: "inside the repository"},
	} {
		problems := test.check.problems(2)
		if len(problems) != 1 || !strings.HasPrefix(problems[0], "path check 2: ") || !strings.Contains(problems[0], test.want) {
			t.Errorf("problems(%#v) = %q, want one naming %q", test.check, problems, test.want)
		}
	}
}
