package orchestrator

import (
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// wordingMessage carries the latest pass that took a turn's language findings
// into the next one. A firing that took no turn cannot consume the reminder.
func wordingMessage(earlier []runstate.Sweep) string {
	for index := len(earlier) - 1; index >= 0; index-- {
		pass := earlier[index]
		if pass.Turns == 0 && len(pass.Wording) == 0 {
			continue
		}
		if len(pass.Wording) == 0 {
			return ""
		}
		var message strings.Builder
		fmt.Fprintf(&message, "\n\n# Words to correct from your previous pass\n\nThe pass of %s used words the terms register does not permit in text for a person. Correct them in your next report and summary; the earlier records are kept as written.\n", pass.Task)
		for index, finding := range pass.Wording {
			if index == maxUntracedListed {
				fmt.Fprintf(&message, "- and %d more; the pass's wording findings are in `yoyo sweeps --json`\n", len(pass.Wording)-index)
				break
			}
			fmt.Fprintf(&message, "- %s\n", cutFinding(finding.Warning()))
		}
		return message.String()
	}
	return ""
}
