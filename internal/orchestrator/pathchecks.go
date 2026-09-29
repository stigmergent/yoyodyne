package orchestrator

import (
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/checks"
	"github.com/mason-bryant/yoyodyne/internal/config"
)

// addedCheck is a path check the per-run gate runs for this change, and why.
type addedCheck struct {
	command string
	reason  string
}

// pathChecksFor chooses which of the configured path checks this change runs:
// each one whose paths file covers a path the change touches, read from the
// worktree so the change is judged by its own copy of the list. A check whose
// list cannot be read runs rather than being passed over, because a gate that
// skipped it would be deciding from a declaration it does not have — and the
// reason says so, so the stage's record names what to fix.
func pathChecksFor(root string, configured []config.PathCheck, changed []string) []addedCheck {
	var added []addedCheck
	for _, check := range configured {
		patterns, err := checks.ReadPathPatterns(root, check.Paths)
		if err != nil {
			added = append(added, addedCheck{
				command: check.Command,
				reason:  "its paths file could not be read, so it runs rather than being passed over: " + err.Error(),
			})
			continue
		}
		if touched, ok := checks.Touching(patterns, changed); ok {
			added = append(added, addedCheck{
				command: check.Command,
				reason:  "the change touches " + touched + ", which " + check.Paths + " lists",
			})
		}
	}
	return added
}

// withPathChecks is the configured checks followed by the ones added for this
// change.
func withPathChecks(configured []string, added []addedCheck) []string {
	commands := append([]string(nil), configured...)
	for _, check := range added {
		commands = append(commands, check.command)
	}
	return commands
}

// describePathChecks says what the gate added, for the stage's record; nothing
// where it added nothing.
func describePathChecks(added []addedCheck) string {
	if len(added) == 0 {
		return ""
	}
	parts := make([]string, 0, len(added))
	for _, check := range added {
		parts = append(parts, check.command+" added because "+check.reason)
	}
	return "; " + strings.Join(parts, "; ")
}
