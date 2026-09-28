// Package maintenancejob is the operator's hand-rolled launchd maintenance job,
// com.yoyodyne.maintenance, as the product sees it: whether it is on this
// machine, what of the product it duplicates, and retiring it.
//
// The job predates the supervisor. Every ten minutes it ran a script that
// started and restarted the scheduler, restarted the Slack sink, started the
// dashboard, ran `yoyo reconcile`, and rebuilt bin/yoyo after a landing. Once
// the supervisor manages those parts, the job manages them too for as long as
// it is loaded, and two managers of one process start, kill, and restart it
// against each other: on 2026-09-26 the job's bounce-when-idle step killed the
// watch 32 times, cancelling the pulls and recurring passes in it. Retiring it
// used to be two commands from docs/operations.md, which is a hand step, and the
// operator's rule of 2026-09-26 is that a hand step is a defect.
//
// So `yoyo doctor` reports the job as a second manager of the product's parts,
// naming what it duplicates, and the supervisor retires it as it is installed:
// the job is booted out of launchd and its property list removed, and what it
// was is recorded. But the retirement does not outrun the replacements: every
// duty the job carried is given an owner in the product (BuildOwners), and a
// job doing anything this build has no owner for is left where it is, with
// what it does and the work that supplies each owner named, rather than turned
// into hand steps. Rebuilding bin/yoyo when the target branch lands is the
// supervisor's own (supervise.Rebuilder).
//
// Only the job this user's LaunchAgents directory installed is retired. A job
// of the same label that launchd loaded from anywhere else is reported and left
// alone, because what the product removes has to be what it can name.
package maintenancejob

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

// Label is the job's launchd label.
const Label = "com.yoyodyne.maintenance"

// launchAgents is where a per-user launchd job's property list lives, under the
// user's home.
const launchAgents = "Library/LaunchAgents"

// launchctlTimeout bounds each question put to launchctl, which answers at once
// or not at all.
const launchctlTimeout = 15 * time.Second

// maxReadBytes bounds what is read of the property list and of the script it
// runs: both are a page, and a file that is not is not the job this knows.
const maxReadBytes = 1 << 20

// Machine is what the job is looked for through. Every field is a seam, so a
// test arranges a loaded job without touching the machine's own launchd.
type Machine struct {
	// Home is the user's home directory, under which the job's property list is.
	Home string
	// UID names the launchd domain the job is loaded in, gui/<uid>.
	UID int
	// GOOS is the platform; launchd is macOS's, and elsewhere there is no job.
	GOOS string
	// Runner asks launchctl.
	Runner execution.ProcessRunner
	// Owners is who carries each duty once the job is gone; nil is this build's
	// own, BuildOwners.
	Owners map[string]Ownership
}

// Duty is one thing the job does that a part of the product does, or has to do
// once the job is gone.
type Duty struct {
	// Part names the product's part it duplicates.
	Part string `json:"part"`
	// Does says what the job does.
	Does string `json:"does"`
	// Owner says who in the product carries the duty once the job is gone, and
	// Pending names the work that has to land before that owner is in this
	// build; an empty Pending is an owner this build has.
	Owner   string `json:"owner"`
	Pending string `json:"pending,omitempty"`
}

// Owned reports a duty this build carries without the job.
func (d Duty) Owned() bool { return d.Pending == "" }

// Describe is the duty and who carries it, as one clause.
func (d Duty) Describe() string {
	if d.Owned() {
		return d.Does + ", which " + d.Owner + " does"
	}
	return fmt.Sprintf("%s, which nothing in this build does: %s does it once %s lands", d.Does, d.Owner, d.Pending)
}

// knownDuties are the job's steps, recognized in its script by the command each
// runs. They are the steps the job had when the supervisor replaced it.
var knownDuties = []struct {
	pattern *regexp.Regexp
	duty    Duty
}{
	{regexp.MustCompile(`work --watch`), Duty{Part: "scheduler", Does: "starts the scheduler (`yoyo work --watch`) where it is not running"}},
	{regexp.MustCompile(`yoyo slack|slack ensure|slack sink`), Duty{Part: "slack", Does: "starts and restarts the Slack sink"}},
	{regexp.MustCompile(`yoyo dashboard`), Duty{Part: "dashboard", Does: "starts the dashboard"}},
	{regexp.MustCompile(`yoyo reconcile`), Duty{Part: "maintenance", Does: "runs `yoyo reconcile` on a schedule"}},
	{regexp.MustCompile(`make build|go build`), Duty{Part: "rebuild", Does: "rebuilds bin/yoyo after a landing"}},
	{regexp.MustCompile(`gh pr merge`), Duty{Part: "courier", Does: "merges the harness's finished pull requests"}},
	{regexp.MustCompile(`triage (repair|rerun)`), Duty{Part: "verification", Does: "carries out the development manager's repair and re-run decisions"}},
	{regexp.MustCompile(`\bkill\b`), Duty{Part: "restart", Does: "kills and restarts a part it judges stalled or older than the binary"}},
}

// Ownership is who in the product carries one of the job's duties, and, where
// that owner is not yet in this build, the work that puts it there.
type Ownership struct {
	Owner   string
	Pending string
}

// BuildOwners is who carries each of the job's duties in this build. The
// build's own table is the deployed build's answer, because the supervisor that
// retires the job runs from the deployed binary. An entry with Pending set is a
// duty nothing here does yet, and the job is not retired while it has one: a
// duty the retirement leaves to a hand is a defect (the operator's rule of
// 2026-09-26). The work that supplies an owner clears its Pending here.
//
// The courier's merges and the verification carry-outs are the harness's own
// already, and are not the supervisor's. Every publishing run asks the forge to
// merge its own pull request, `yoyo reconcile` makes the merge request for a
// promoted run whose record holds none, and a publication nothing asked the
// forge to merge is docketed for the development manager, whose re-arm the watch
// carries out; the courier's direct merge of a mergeable request is a bypass of
// the merge queue the target branch now has. The verification pass asked the
// development manager to audit her sweep and then fired the repairs and re-runs
// it named; the watch fires every recorded repair, re-run, and re-arm itself on
// its next pull, and a sweep that stops passing is a missed pass on
// `yoyo sweeps` and a failing task on `yoyo status`.
func BuildOwners() map[string]Ownership {
	return map[string]Ownership{
		"scheduler":    {Owner: "the supervisor's scheduler child"},
		"slack":        {Owner: "the supervisor's slack child"},
		"rebuild":      {Owner: "the supervisor's rebuild"},
		"courier":      {Owner: "the harness's own publication (each run's merge request, `yoyo reconcile`'s merge request for a promotion that has none, and the watch's carry-out of a re-arm)"},
		"verification": {Owner: "the watch, which carries out every recorded repair, re-run, and re-arm on its next pull"},
		"dashboard":    {Owner: "the supervisor's dashboard child", Pending: "yoyodyne-ifd.414"},
		"maintenance":  {Owner: "the supervisor's periodic pass", Pending: "yoyodyne-ifd.413"},
		"restart":      {Owner: "the supervisor's own pass", Pending: "yoyodyne-ifd.434.3"},
	}
}

// Found is the job as this machine has it.
type Found struct {
	// Plist is where this user's job's property list is, and Installed whether
	// it is there.
	Plist     string `json:"plist"`
	Installed bool   `json:"installed"`
	// Loaded is launchd holding a job of this label in the user's domain, and
	// LoadedFrom the property list launchd says it loaded it from.
	Loaded     bool   `json:"loaded"`
	LoadedFrom string `json:"loaded_from,omitempty"`
	// Unasked is why launchctl could not be asked, where it could not; whether
	// the job is loaded is then not known.
	Unasked string `json:"unasked,omitempty"`
	// Content is the property list as read, and Program what it runs.
	Content string   `json:"-"`
	Program []string `json:"program,omitempty"`
	// Duplicates is what the job does that the product does, read from its
	// script; DutiesInferred says the script could not be read, so what is named
	// is what the job is known to do rather than what this one was seen to.
	Duplicates     []Duty `json:"duplicates,omitempty"`
	DutiesInferred bool   `json:"duties_inferred,omitempty"`
}

// Present reports a job on this machine in either form: loaded, or installed to
// be loaded at the next login.
func (f Found) Present() bool { return f.Installed || f.Loaded }

// Ours reports the loaded job as the one this user's LaunchAgents installed,
// which is the only one the product retires. A job launchd names no source for
// is taken as ours only where the property list is there to say so.
func (f Found) Ours() bool {
	if !f.Loaded {
		return f.Installed
	}
	if f.LoadedFrom == "" {
		return f.Installed
	}
	return samePath(f.LoadedFrom, f.Plist)
}

// Domain is the launchd service target the job is addressed by.
func (m Machine) Domain() string { return fmt.Sprintf("gui/%d/%s", m.UID, Label) }

// PlistPath is where this user's job's property list is.
func (m Machine) PlistPath() string {
	return filepath.Join(m.Home, filepath.FromSlash(launchAgents), Label+".plist")
}

// Inspect looks for the job. It changes nothing. A platform without launchd
// has no job, and says so by finding nothing.
func (m Machine) Inspect(ctx context.Context) (Found, error) {
	found := Found{}
	if m.GOOS != "darwin" {
		return found, nil
	}
	if m.Home == "" {
		return found, errors.New("the user's home directory is not known, so the job's property list cannot be looked for")
	}
	found.Plist = m.PlistPath()
	content, err := readBounded(found.Plist)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return found, fmt.Errorf("read %s: %w", found.Plist, err)
	default:
		found.Installed = true
		found.Content = string(content)
		found.Program = programArguments(content)
	}
	m.askLaunchd(ctx, &found)
	if found.Present() {
		found.Duplicates, found.DutiesInferred = duties(found.Program)
		owners := m.Owners
		if owners == nil {
			owners = BuildOwners()
		}
		for index, duty := range found.Duplicates {
			ownership, known := owners[duty.Part]
			if !known {
				ownership = Ownership{Owner: "nothing the product names", Pending: "an owner being named for it"}
			}
			found.Duplicates[index].Owner, found.Duplicates[index].Pending = ownership.Owner, ownership.Pending
		}
	}
	return found, nil
}

// Unowned is what the job does that nothing in this build does, which
// retiring it would leave to a hand.
func (f Found) Unowned() []Duty {
	var unowned []Duty
	for _, duty := range f.Duplicates {
		if !duty.Owned() {
			unowned = append(unowned, duty)
		}
	}
	return unowned
}

// askLaunchd asks whether a job of the label is loaded in the user's domain,
// and where launchd loaded it from.
func (m Machine) askLaunchd(ctx context.Context, found *Found) {
	if m.Runner == nil {
		found.Unasked = "no process runner was wired to ask launchctl"
		return
	}
	result, err := m.Runner.Run(ctx, execution.Command{
		Name:    "launchctl",
		Args:    []string{"print", m.Domain()},
		Timeout: launchctlTimeout,
	}, nil)
	if err != nil {
		found.Unasked = fmt.Sprintf("launchctl could not be asked: %v", err)
		return
	}
	if result.Status != execution.ProcessSucceeded {
		// launchctl print answers a label it does not hold with a failure, which
		// is the ordinary answer: nothing is loaded.
		return
	}
	found.Loaded = true
	found.LoadedFrom = loadedPath(result.Stdout)
}

// Retirement is what Retire did.
type Retirement struct {
	Found Found `json:"found"`
	// Unloaded is the job booted out of launchd, and Removed its property list
	// removed.
	Unloaded bool `json:"unloaded"`
	Removed  bool `json:"removed"`
	// Left says what was not touched, and why.
	Left string `json:"left,omitempty"`
	// Held is what the job does that nothing in this build does; a job with any
	// is left where it is, because retiring it would leave those to a hand.
	Held []Duty `json:"held,omitempty"`
}

// Acted reports a retirement that changed anything.
func (r Retirement) Acted() bool { return r.Unloaded || r.Removed }

// Retire unloads the job and removes its property list, where this user's
// LaunchAgents installed it. A job that is not there is nothing to do, and
// repeating a retirement does nothing. A job of the label loaded from anywhere
// else is left loaded and said so, because it is not the one the product can
// name. A job doing anything this build has no owner for is left as it is,
// with those duties named under Held: the retirement does not outrun the
// replacements.
func (m Machine) Retire(ctx context.Context) (Retirement, error) {
	found, err := m.Inspect(ctx)
	retirement := Retirement{Found: found}
	if err != nil {
		return retirement, err
	}
	if !found.Present() {
		return retirement, nil
	}
	if held := found.Unowned(); len(held) > 0 && found.Ours() {
		retirement.Held = held
		return retirement, nil
	}
	if found.Loaded && !found.Ours() {
		retirement.Left = fmt.Sprintf("launchd loaded %s from %s rather than from %s, so it is not the job this user's LaunchAgents installed and is left loaded", Label, firstNonEmpty(found.LoadedFrom, "a property list it did not name"), found.Plist)
	} else if found.Loaded {
		result, runErr := m.Runner.Run(ctx, execution.Command{
			Name:    "launchctl",
			Args:    []string{"bootout", m.Domain()},
			Timeout: launchctlTimeout,
		}, nil)
		if runErr == nil && result.Status == execution.ProcessSucceeded {
			retirement.Unloaded = true
		} else {
			// A bootout that failed because the job had already gone is a bootout
			// made; asking again is what tells the two apart.
			again := Found{}
			m.askLaunchd(ctx, &again)
			if again.Unasked != "" || again.Loaded {
				return retirement, fmt.Errorf("boot %s out of launchd: %s", m.Domain(), describe(result, runErr))
			}
			retirement.Unloaded = true
		}
	}
	if found.Installed {
		root, err := repowrite.NewRoot(filepath.Dir(found.Plist))
		if err != nil {
			return retirement, fmt.Errorf("remove %s: %w", found.Plist, err)
		}
		if _, err := root.RemoveFile(filepath.Base(found.Plist)); err != nil {
			return retirement, fmt.Errorf("remove %s: %w", found.Plist, err)
		}
		retirement.Removed = true
	}
	if script := scriptOf(found.Program); script != "" && retirement.Left == "" {
		retirement.Left = fmt.Sprintf("the job's script %s is the operator's file and is left where it is; nothing runs it any more", script)
	}
	return retirement, nil
}

// Describe is what a retirement did, in a sentence.
func (r Retirement) Describe() string {
	var done []string
	if r.Unloaded {
		done = append(done, "booted it out of launchd")
	}
	if r.Removed {
		done = append(done, "removed "+r.Found.Plist)
	}
	said := fmt.Sprintf("retired the operator's maintenance job %s, a second manager of the product's parts: %s", Label, strings.Join(done, " and "))
	if parts := DescribeDuties(r.Found.Duplicates); parts != "" {
		said += "; it " + parts
	}
	return said
}

// DescribeHeld is why a job with duties this build has no owner for was not
// retired, naming each of them and the work that supplies its owner.
func (r Retirement) DescribeHeld() string {
	if len(r.Held) == 0 {
		return ""
	}
	return fmt.Sprintf("the operator's maintenance job %s was not retired: %d of its duties have no owner in this build, and retiring it would leave them to a hand — it %s; it goes on running beside the supervisor, and the first start of a build that owns every duty retires it", Label, len(r.Held), DescribeDuties(r.Held))
}

// DescribeDuties is what the job duplicates and who carries each, as one
// clause.
func DescribeDuties(duties []Duty) string {
	var said []string
	for _, duty := range duties {
		said = append(said, duty.Describe())
	}
	return strings.Join(said, "; ")
}

// DutyNames is the parts the job duplicates, by name.
func DutyNames(duties []Duty) []string {
	var names []string
	for _, duty := range duties {
		names = append(names, duty.Part)
	}
	return names
}

// duties reads what the job does out of the script it runs. A script that
// cannot be read is answered with every duty the job is known to have had,
// marked as inferred, because a job nobody can read is still a job that was
// doing all of it.
func duties(program []string) ([]Duty, bool) {
	script := scriptOf(program)
	if script == "" {
		return allDuties(), true
	}
	content, err := readBounded(script)
	if err != nil {
		return allDuties(), true
	}
	seen := make(map[string]bool, len(knownDuties))
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64<<10), maxReadBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		for _, known := range knownDuties {
			if known.pattern.MatchString(line) {
				seen[known.duty.Part] = true
			}
		}
	}
	var found []Duty
	for _, known := range knownDuties {
		if seen[known.duty.Part] {
			found = append(found, known.duty)
		}
	}
	return found, false
}

func allDuties() []Duty {
	all := make([]Duty, 0, len(knownDuties))
	for _, known := range knownDuties {
		all = append(all, known.duty)
	}
	return all
}

// scriptOf is the file the job runs: the last argument that names an absolute
// path, which is the script whether the job runs it directly or through a shell.
func scriptOf(program []string) string {
	for index := len(program) - 1; index >= 0; index-- {
		if filepath.IsAbs(program[index]) && !strings.HasSuffix(program[index], "/sh") && !strings.HasSuffix(program[index], "/bash") && !strings.HasSuffix(program[index], "/zsh") {
			return program[index]
		}
	}
	return ""
}

// programArguments reads ProgramArguments out of a property list: the array of
// strings that follows that key in the top-level dictionary, or Program where
// the list names a single one.
func programArguments(content []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	var (
		lastKey  string
		inKey    bool
		inArray  bool
		inString bool
		wanted   string
		program  []string
		text     strings.Builder
	)
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "key":
				inKey = true
				text.Reset()
			case "array":
				if lastKey == "ProgramArguments" {
					inArray = true
					wanted = "ProgramArguments"
				}
			case "string":
				inString = true
				text.Reset()
			}
		case xml.CharData:
			if inKey || inString {
				text.Write(typed)
			}
		case xml.EndElement:
			switch typed.Name.Local {
			case "key":
				inKey = false
				lastKey = strings.TrimSpace(text.String())
			case "array":
				if inArray {
					return program
				}
			case "string":
				inString = false
				value := strings.TrimSpace(text.String())
				switch {
				case inArray && wanted == "ProgramArguments":
					program = append(program, value)
				case lastKey == "Program":
					program = []string{value}
				}
				if !inArray {
					lastKey = ""
				}
			}
		}
	}
	return program
}

// loadedPath reads the property list launchd says it loaded a service from,
// the `path = ` line of `launchctl print`.
func loadedPath(printed string) string {
	for _, line := range strings.Split(printed, "\n") {
		trimmed := strings.TrimSpace(line)
		if value, found := strings.CutPrefix(trimmed, "path = "); found {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxReadBytes))
}

func samePath(first, second string) bool {
	if filepath.Clean(first) == filepath.Clean(second) {
		return true
	}
	resolvedFirst, errFirst := filepath.EvalSymlinks(first)
	resolvedSecond, errSecond := filepath.EvalSymlinks(filepath.Dir(second))
	if errFirst != nil || errSecond != nil {
		return false
	}
	return filepath.Clean(resolvedFirst) == filepath.Join(resolvedSecond, filepath.Base(second))
}

func describe(result execution.ProcessResult, err error) string {
	if err != nil {
		return err.Error()
	}
	said := strings.TrimSpace(firstNonEmpty(result.Stderr, result.Stdout))
	if said == "" {
		return fmt.Sprintf("launchctl exited %d", result.ExitCode)
	}
	return fmt.Sprintf("launchctl exited %d: %s", result.ExitCode, said)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
