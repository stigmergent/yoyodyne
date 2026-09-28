package backend

// How hard an agent's provider is asked to think on each invocation.
//
// Claude Code takes an effort level on its command line, and a session that is
// given none runs at whatever the provider resolves on its own: an environment
// variable, a level saved in the machine's settings, or the model's default,
// which differs by model. None of those is written down anywhere the harness
// reads, so a role that names no level runs at a level nobody chose and no
// record says. The configuration therefore names one beside the model, the
// adapter passes it on every invocation, and the record says what was asked.
//
// Which levels a provider accepts is the descriptor's, as which roles it serves
// is, so what refuses a configuration and what an adapter would pass are one
// statement. Codex accepts none here. It has a reasoning-effort setting of its
// own, but its accepted values and its default could not be confirmed against
// its documentation or observed from its binary when this was written, and a
// level passed to a provider that rejects it fails the invocation rather than
// the configuration. So a Codex agent naming a level is refused when the file
// loads, naming why, rather than launched with a guess.

import "strings"

// claudeCodeEffortLevels are the levels Claude Code's --effort flag accepts, as
// its own help states them: "Effort level for the current session (low, medium,
// high, xhigh, max)".
var claudeCodeEffortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// AcceptsEffort reports whether this provider accepts the named level.
func (d Descriptor) AcceptsEffort(level string) bool {
	level = strings.TrimSpace(level)
	for _, accepted := range d.EffortLevels {
		if accepted == level {
			return true
		}
	}
	return false
}

// DescribeEffortLevels names the levels this provider accepts the way a refusal
// says them: "low, medium, high, xhigh, or max", or that it accepts none.
func (d Descriptor) DescribeEffortLevels() string {
	switch len(d.EffortLevels) {
	case 0:
		return "no effort level"
	case 1:
		return d.EffortLevels[0]
	}
	return strings.Join(d.EffortLevels[:len(d.EffortLevels)-1], ", ") + ", or " + d.EffortLevels[len(d.EffortLevels)-1]
}
