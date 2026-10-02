package backend

import (
	"errors"
	"time"
)

const CapacityProbeTimeout = 30 * time.Second
const CapacityProbePrompt = "Reply with OK to confirm this endpoint can serve a request. Do not inspect files or use tools."

// RestrictCapacityProbe applies the probe contract inside each compiled adapter,
// before any process starts. Ordinary requests keep their role's posture; a
// probe gets a fixed prompt and bound, no tool grants, and no continuing session.
// Claude Code disables tools; Codex holds its native read-only sandbox.
func (r RunRequest) RestrictCapacityProbe() (RunRequest, error) {
	if !r.CapacityProbe {
		return r, nil
	}
	if r.SessionID != "" || len(r.AllowedTools) != 0 {
		return RunRequest{}, errors.New("capacity probes cannot resume a session or be granted tools")
	}
	r.Prompt = CapacityProbePrompt
	r.SystemPrompt = ""
	r.AllowedTools = []string{}
	r.Timeout = CapacityProbeTimeout
	r.IdleTimeout = CapacityProbeTimeout
	return r, nil
}
