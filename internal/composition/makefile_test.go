package composition

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the repository's target over scratch input, so the recipe rather than a
// copy of its shell logic is what these cases hold to account.
func TestFmtcheck(t *testing.T) {
	t.Parallel()

	requireTool(t, "make")
	requireTool(t, "gofmt")
	makefile, err := filepath.Abs(filepath.Join(repositoryRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		source      string
		formatter   string
		wantFailure bool
		wantStdout  string
		wantStderr  []string
	}{
		{
			name:   "formatted",
			source: "package fixture\n\nvar answer = 42\n",
		},
		{
			name:        "unformatted",
			source:      "package fixture\nvar answer=42\n",
			wantFailure: true,
			wantStdout:  "gofmt needed:\nfixture.go\n",
		},
		{
			name:        "malformed",
			source:      "package fixture\n\nfunc broken(\n",
			wantFailure: true,
			wantStderr:  []string{"fixture.go:", "expected"},
		},
		{
			name:        "formatter failure with diagnostics and empty stdout",
			source:      "package fixture\n",
			formatter:   "#!/bin/sh\nprintf '%s\\n' 'formatter could not read input' >&2\nexit 23\n",
			wantFailure: true,
			wantStderr:  []string{"formatter could not read input", "Error 23"},
		},
		{
			name:        "formatter failure with no output",
			source:      "package fixture\n",
			formatter:   "#!/bin/sh\nexit 29\n",
			wantFailure: true,
			wantStderr:  []string{"Error 29"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeFixture(t, map[string]string{"fixture.go": test.source})
			command := exec.Command("make", "--no-print-directory", "-f", makefile, "fmtcheck", "VERSION=fmtcheck-test")
			command.Dir = root
			command.Env = append(os.Environ(), "MAKEFLAGS=", "MFLAGS=", "MAKELEVEL=0")
			if test.formatter != "" {
				tools := filepath.Join(root, "tools")
				if err := os.Mkdir(tools, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(tools, "gofmt"), []byte(test.formatter), 0o755); err != nil {
					t.Fatal(err)
				}
				command.Env = append(command.Env, "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"))
			}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			if test.wantFailure {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Errorf("fmtcheck error = %v, want a nonzero exit; stdout:\n%s\nstderr:\n%s", err, &stdout, &stderr)
				}
			} else if err != nil {
				t.Errorf("fmtcheck error = %v; stdout:\n%s\nstderr:\n%s", err, &stdout, &stderr)
			}
			if stdout.String() != test.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), test.wantStdout)
			}
			for _, diagnostic := range test.wantStderr {
				if !strings.Contains(stderr.String(), diagnostic) {
					t.Errorf("stderr = %q, want it to contain %q", stderr.String(), diagnostic)
				}
			}
			if !test.wantFailure && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want no diagnostics for formatted input", stderr.String())
			}
		})
	}
}
