package config

import (
	"strings"
	"testing"
)

// Where the harness keeps its state describes one machine, and a project file is
// committed and read on every machine that checks it out, so a project file that
// sets the state root is refused by name, with where the key belongs.
func TestAStateRootInAProjectConfigurationIsRefusedByName(t *testing.T) {
	t.Parallel()

	for key, content := range map[string]string{
		"state_root":           minimalProjectConfig + "state_root: /var/yoyodyne\n",
		"execution.state_root": minimalProjectConfig + "execution:\n  state_root: /var/yoyodyne\n",
	} {
		_, err := loadProjectError(t, content, nil)
		if err == nil {
			t.Fatalf("LoadResolved() accepted a project file setting %s", key)
		}
		for _, want := range []string{key + " is not a project setting", MachineFileName, "YOYODYNE_STATE_HOME"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s refusal %q does not say %q", key, err, want)
			}
		}
	}
}
