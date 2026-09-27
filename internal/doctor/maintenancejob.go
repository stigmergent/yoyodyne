package doctor

// Whether a launchd job outside the product manages the product's parts.
//
// The operator's maintenance job, com.yoyodyne.maintenance, started and
// restarted the scheduler and the sink, started the dashboard, ran
// `yoyo reconcile`, and rebuilt bin/yoyo, every ten minutes, from a script
// outside the repository. The supervisor does those now, and a job still loaded
// beside it is a second manager of the same processes: the two start, kill, and
// restart them against each other, which is what cancelled a program manager's
// first pass on 2026-09-26. The finding names the job and what it duplicates,
// and its remedy is `yoyo start`, which retires the job as it installs the
// supervisor.
//
// It is a warning rather than a problem: no run is stopped by it, only bounced.

import (
	"context"
	"fmt"
	"os"

	"github.com/mason-bryant/yoyodyne/internal/maintenancejob"
)

const maintenanceJobCheck = "maintenance-job"

// checkMaintenanceJob reports the operator's maintenance launchd job where this
// machine has it. It asks launchctl and reads the job's files, and changes
// nothing. A platform without launchd has nothing to report.
func (d *diagnosis) checkMaintenanceJob(ctx context.Context) []Finding {
	if d.env.GOOS != "darwin" {
		return nil
	}
	home, err := d.homeDir()
	if err != nil {
		return []Finding{{
			Check:   maintenanceJobCheck,
			Status:  StatusWarning,
			Summary: "whether a launchd job outside the product manages its parts could not be asked: the home directory is not known",
			Detail:  err.Error(),
			Remedy:  "export HOME=/Users/$(id -un)",
		}}
	}
	machine := maintenancejob.Machine{Home: home, UID: os.Getuid(), GOOS: d.env.GOOS, Runner: d.env.Runner}
	found, err := machine.Inspect(ctx)
	if err != nil {
		return []Finding{{
			Check:   maintenanceJobCheck,
			Status:  StatusWarning,
			Summary: fmt.Sprintf("whether the launchd job %s manages the product's parts could not be asked", maintenancejob.Label),
			Detail:  err.Error(),
			Remedy:  fmt.Sprintf("launchctl print %s", machine.Domain()),
		}}
	}
	if !found.Present() {
		finding := Finding{
			Check:   maintenanceJobCheck,
			Status:  StatusOK,
			Summary: fmt.Sprintf("no launchd job outside the product manages its parts: %s is neither loaded nor installed", maintenancejob.Label),
		}
		if found.Unasked != "" {
			finding.Summary = fmt.Sprintf("%s is not installed; whether launchd holds it could not be asked", maintenancejob.Label)
			finding.Detail = found.Unasked
		}
		return []Finding{finding}
	}
	state := "is loaded"
	if !found.Loaded {
		state = "is installed and loads at the next login"
	}
	detail := "it " + maintenancejob.DescribeDuties(found.Duplicates)
	if found.DutiesInferred {
		detail += " (its script could not be read, so this is what the job is known to do)"
	}
	detail += fmt.Sprintf("; two managers of one part start, kill, and restart it against each other. Its property list is %s", found.Plist)
	if found.LoadedFrom != "" && found.LoadedFrom != found.Plist {
		detail += fmt.Sprintf(", and launchd loaded it from %s", found.LoadedFrom)
	}
	remedy := "yoyo start"
	if found.Loaded && !found.Ours() {
		// Not the job this user's LaunchAgents installed, so not one the
		// supervisor retires; unloading it is the command left.
		remedy = fmt.Sprintf("launchctl bootout %s", machine.Domain())
	} else {
		detail += "; `yoyo start` retires it as it installs the supervisor, records what it was, and leaves its script where it is"
	}
	return []Finding{{
		Check:   maintenanceJobCheck,
		Status:  StatusWarning,
		Summary: fmt.Sprintf("the launchd job %s %s, a second manager of the product's parts beside the supervisor", maintenancejob.Label, state),
		Detail:  detail,
		Remedy:  remedy,
	}}
}
