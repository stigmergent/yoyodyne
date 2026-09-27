package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installMaintenanceJob puts the operator's job under the world's home: its
// property list, and the script it runs.
func (w *world) installMaintenanceJob() string {
	w.t.Helper()
	script := filepath.Join(w.project, ".local", "yoyodyne", "yoyodyne-maintenance.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		w.t.Fatal(err)
	}
	body := "#!/bin/bash\nbin/yoyo reconcile\nmake build\nnohup bin/yoyo work --watch &\nbin/yoyo slack ensure\nnohup bin/yoyo dashboard -port 8080 &\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		w.t.Fatal(err)
	}
	plist := filepath.Join(w.project, "Library", "LaunchAgents", "com.yoyodyne.maintenance.plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		w.t.Fatal(err)
	}
	content := "<plist version=\"1.0\"><dict><key>Label</key><string>com.yoyodyne.maintenance</string><key>ProgramArguments</key><array><string>" + script + "</string></array></dict></plist>\n"
	if err := os.WriteFile(plist, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
	return plist
}

// A loaded maintenance job is a second manager of the product's parts: the
// finding names it and every part it duplicates, and its remedy is the verb
// that retires it.
func TestTheDoctorReportsALoadedMaintenanceJobAsASecondManager(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	plist := w.installMaintenanceJob()
	w.runner.reply("launchctl print", succeeded("gui/501/com.yoyodyne.maintenance = {\n\tpath = "+plist+"\n}\n"))
	report := w.diagnose()
	finding, found := findingFor(report, "maintenance-job")
	if !found {
		t.Fatalf("no maintenance-job finding:%s", render(report))
	}
	if finding.Status != StatusWarning || finding.Remedy != "yoyo start" {
		t.Errorf("finding = %+v, want a warning whose remedy is `yoyo start`", finding)
	}
	if !strings.Contains(finding.Summary, "com.yoyodyne.maintenance is loaded") || !strings.Contains(finding.Summary, "second manager") {
		t.Errorf("summary = %q, want the job named as loaded and as a second manager", finding.Summary)
	}
	for _, want := range []string{"scheduler", "Slack sink", "dashboard", "yoyo reconcile", "rebuilds bin/yoyo", plist} {
		if !strings.Contains(finding.Detail, want) {
			t.Errorf("detail = %q, want %q named in it", finding.Detail, want)
		}
	}
	if !report.Healthy() {
		t.Errorf("a maintenance job made the installation unhealthy:%s", render(report))
	}
}

// No job is a healthy line, and an installed job launchd has not loaded yet is
// still the finding, since it loads at the next login.
func TestTheDoctorSaysWhenNoMaintenanceJobIsThereAndWhenOneWaitsForLogin(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	finding, found := findingFor(w.diagnose(), "maintenance-job")
	if !found || finding.Status != StatusOK {
		t.Fatalf("finding = %+v, %t, want a healthy line with no job", finding, found)
	}

	w.installMaintenanceJob()
	finding, _ = findingFor(w.diagnose(), "maintenance-job")
	if finding.Status != StatusWarning || !strings.Contains(finding.Summary, "next login") {
		t.Errorf("finding = %+v, want a warning that the job loads at the next login", finding)
	}
}

// launchd is macOS's, and elsewhere nothing is asked.
func TestTheDoctorAsksNoLaunchdOffMacOS(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	w.goos = "linux"
	if _, found := findingFor(w.diagnose(), "maintenance-job"); found {
		t.Error("a maintenance-job finding was made on a platform without launchd")
	}
}
