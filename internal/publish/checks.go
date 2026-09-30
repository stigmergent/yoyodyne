package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

const (
	// maxReadCheckRuns bounds the check runs one reading takes for a head. The
	// forge pages at a hundred, and a head with more than that is a repository
	// whose checks are not what this reading is for.
	maxReadCheckRuns = 100
	// maxAnnotatedChecks bounds how many failing checks have their annotations
	// read, because each one is a call of its own. A request failing more than
	// this many checks is red whichever files they name.
	maxAnnotatedChecks = 10
	// maxAnnotationsRead bounds the annotations read for one failing check.
	maxAnnotationsRead = 50
	// maxAnnotationsKept bounds the annotations a failing check carries whole —
	// path, line, level, and the forge's own message — beside every file named.
	maxAnnotationsKept = 10
)

// ErrForgeAccessRefused is the forge refusing the harness's token something it
// asked for — reading a job's log, or running a job again — as opposed to the
// thing not being there. Granting the token that access is the operator's, and
// whoever reads the refusal is told so rather than being sent to fetch it.
var ErrForgeAccessRefused = errors.New("the forge refused the harness's token access")

// accessRefused recognizes the forge's refusal of a token's permissions in what
// gh printed: GitHub answers 403 with "Resource not accessible by integration"
// for an app or fine-grained token without the scope, and "Must have admin
// rights" where the job needs more than read.
func accessRefused(stderr string) bool {
	lowered := strings.ToLower(stderr)
	return strings.Contains(lowered, "http 403") ||
		strings.Contains(lowered, "resource not accessible") ||
		strings.Contains(lowered, "must have admin rights")
}

// forgeFailure is the error for a forge call that ran and was refused, marked
// ErrForgeAccessRefused where the refusal was of the token's access.
func (g GitHub) forgeFailure(what string, result execution.ProcessResult) error {
	stderr := g.redact(firstLine(strings.TrimSpace(result.Stderr)))
	if accessRefused(result.Stderr) {
		return fmt.Errorf("%s: exit code %d: %s: %w", what, result.ExitCode, stderr, ErrForgeAccessRefused)
	}
	return fmt.Errorf("%s: exit code %d: %s", what, result.ExitCode, stderr)
}

// Annotation is one annotation the forge made on a check run: where, how
// severe, and the forge's own words, which for a failed step is usually the
// test or the command that failed.
type Annotation struct {
	Path    string
	Line    int
	Level   string
	Message string
}

// FailedCheck is one check the forge reports failing on a pull request's head,
// with the files its annotations name. The files are what says whose failure it
// is: a check whose annotations point only at files the change does not touch
// failed on something the change did not bring, which is what a head behind its
// base branch meets when the base has since been fixed.
//
// A check that annotates nothing names no files, and that is recorded as it is
// rather than guessed at: nothing here says whose failure it is.
//
// ID is the forge's check run, which for an Actions job is the job a re-run
// names. Conclusion is how the forge says the check ended: a job cancelled,
// timed out, or never started failed in the runner rather than on any file.
// URL is the forge's page for the check run, which for an Actions job is its
// log. Annotations are the forge's own account of the failure, bounded.
type FailedCheck struct {
	Name        string
	Paths       []string
	ID          int64
	Conclusion  string
	URL         string
	Annotations []Annotation
}

// CheckReading is what the forge says about a pull request's checks at the
// moment it was asked: the head the checks ran on, the files the change
// touches, each check's standing, and how far the base branch has moved on
// without the head.
type CheckReading struct {
	HeadCommit string
	// Files are the files the pull request's change touches, as the forge lists
	// them.
	Files   []string
	Failing []FailedCheck
	// Pending names the checks that have not finished.
	Pending []string
	Passing int
	// BehindBy is how many commits the base branch carries that the head does
	// not. Zero is a head level with its base.
	BehindBy int
}

// failingConclusions are the forge's conclusions for a check that did not pass.
// A cancelled or timed-out check is failing here: a required check that ended
// either way holds the merge exactly as a failure does.
var failingConclusions = map[string]bool{
	"failure":         true,
	"timed_out":       true,
	"cancelled":       true,
	"action_required": true,
	"startup_failure": true,
}

// Checks reads a pull request's check state: the head it carries and the
// files its change touches, every check run on that head and how each ended,
// the files each failing check's annotations name, and how far base has moved
// ahead of the head.
//
// It is several reads rather than one, and any of them failing fails the
// reading: a caller deciding whether to withdraw a queued merge or replay a
// change is gating on this, and a reading with a hole in it must refuse rather
// than read as a head with nothing wrong.
func (g GitHub) Checks(ctx context.Context, number int, base string) (CheckReading, error) {
	if number <= 0 {
		return CheckReading{}, fmt.Errorf("pull request number %d is not a request", number)
	}
	if err := validateArgument("base branch", base); err != nil {
		return CheckReading{}, err
	}
	scope, err := g.repoArgs(ctx)
	if err != nil {
		return CheckReading{}, err
	}
	viewed, err := g.exec(ctx, append([]string{"pr", "view", strconv.Itoa(number)}, append(scope, "--json", "headRefOid,files")...)...)
	if err != nil {
		return CheckReading{}, fmt.Errorf("ask the forge about the head of pull request %d: %w", number, err)
	}
	if viewed.Status != execution.ProcessSucceeded {
		return CheckReading{}, fmt.Errorf("ask the forge about the head of pull request %d: exit code %d: %s",
			number, viewed.ExitCode, g.redact(strings.TrimSpace(viewed.Stderr)))
	}
	var request struct {
		HeadRefOid string `json:"headRefOid"`
		Files      []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(viewed.Stdout)), &request); err != nil {
		return CheckReading{}, fmt.Errorf("decode the head of pull request %d: %w", number, err)
	}
	head := strings.TrimSpace(request.HeadRefOid)
	if !commitPattern.MatchString(head) {
		return CheckReading{}, fmt.Errorf("the forge named %q as the head of pull request %d, which is not a commit", head, number)
	}
	reading := CheckReading{HeadCommit: head}
	for _, file := range request.Files {
		if path := strings.TrimSpace(file.Path); path != "" {
			reading.Files = append(reading.Files, path)
		}
	}

	runs, err := g.api(ctx, "repos/{owner}/{repo}/commits/"+head+"/check-runs?per_page="+strconv.Itoa(maxReadCheckRuns))
	if err != nil {
		return CheckReading{}, fmt.Errorf("ask the forge for the checks on %s: %w", head, err)
	}
	if runs.Status != execution.ProcessSucceeded {
		return CheckReading{}, fmt.Errorf("ask the forge for the checks on %s: exit code %d: %s",
			head, runs.ExitCode, g.redact(firstLine(strings.TrimSpace(runs.Stderr))))
	}
	var reported struct {
		CheckRuns []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"check_runs"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(runs.Stdout)), &reported); err != nil {
		return CheckReading{}, fmt.Errorf("decode the checks on %s: %w", head, err)
	}
	for _, run := range reported.CheckRuns {
		name := strings.TrimSpace(run.Name)
		switch {
		case !strings.EqualFold(run.Status, "completed"):
			reading.Pending = append(reading.Pending, name)
		case failingConclusions[strings.ToLower(strings.TrimSpace(run.Conclusion))]:
			failed := FailedCheck{Name: name, ID: run.ID, Conclusion: strings.ToLower(strings.TrimSpace(run.Conclusion)), URL: strings.TrimSpace(run.HTMLURL)}
			if len(reading.Failing) < maxAnnotatedChecks && run.ID > 0 {
				paths, annotations, err := g.annotations(ctx, run.ID)
				if err != nil {
					return CheckReading{}, fmt.Errorf("ask the forge which files check %q annotates: %w", name, err)
				}
				failed.Paths = paths
				failed.Annotations = annotations
			}
			reading.Failing = append(reading.Failing, failed)
		default:
			reading.Passing++
		}
	}

	// How far the base has moved on without the head is the comparison of the
	// head with the base: what the base carries past where the two met.
	compared, err := g.api(ctx, "repos/{owner}/{repo}/compare/"+head+"..."+base)
	if err != nil {
		return CheckReading{}, fmt.Errorf("compare %s with %s: %w", head, base, err)
	}
	if compared.Status != execution.ProcessSucceeded {
		return CheckReading{}, fmt.Errorf("compare %s with %s: exit code %d: %s",
			head, base, compared.ExitCode, g.redact(firstLine(strings.TrimSpace(compared.Stderr))))
	}
	var distance struct {
		AheadBy *int `json:"ahead_by"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(compared.Stdout)), &distance); err != nil {
		return CheckReading{}, fmt.Errorf("decode the comparison of %s with %s: %w", head, base, err)
	}
	if distance.AheadBy == nil {
		return CheckReading{}, fmt.Errorf("the comparison of %s with %s did not say how far %s has moved on", head, base, base)
	}
	reading.BehindBy = *distance.AheadBy
	return reading, nil
}

// BranchCheckReading is how the forge says each check ended on the commit a
// branch points at: the commit, and each check's standing by name. It is what
// says whether a check red on a pull request whose head is level with the branch
// is red on the branch itself, which is the only thing that makes it the
// branch's failure rather than the change's (yoyodyne-c02).
type BranchCheckReading struct {
	HeadCommit string
	// Failing, Passing, and Pending name the checks that failed, passed, and
	// have not finished on the branch's head.
	Failing []string
	Passing []string
	Pending []string
}

// BranchChecks reads how every check ended on the commit a branch points at.
// A branch whose head no check has run on reads as no checks at all, which
// confirms nothing about any of them.
func (g GitHub) BranchChecks(ctx context.Context, branch string) (BranchCheckReading, error) {
	if err := validateArgument("branch", branch); err != nil {
		return BranchCheckReading{}, err
	}
	runs, err := g.api(ctx, "repos/{owner}/{repo}/commits/"+branch+"/check-runs?per_page="+strconv.Itoa(maxReadCheckRuns))
	if err != nil {
		return BranchCheckReading{}, fmt.Errorf("ask the forge for the checks on %s: %w", branch, err)
	}
	if runs.Status != execution.ProcessSucceeded {
		return BranchCheckReading{}, fmt.Errorf("ask the forge for the checks on %s: exit code %d: %s",
			branch, runs.ExitCode, g.redact(firstLine(strings.TrimSpace(runs.Stderr))))
	}
	var reported struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(runs.Stdout)), &reported); err != nil {
		return BranchCheckReading{}, fmt.Errorf("decode the checks on %s: %w", branch, err)
	}
	var reading BranchCheckReading
	for _, run := range reported.CheckRuns {
		if head := strings.TrimSpace(run.HeadSHA); reading.HeadCommit == "" && commitPattern.MatchString(head) {
			reading.HeadCommit = head
		}
		name := strings.TrimSpace(run.Name)
		switch {
		case !strings.EqualFold(run.Status, "completed"):
			reading.Pending = append(reading.Pending, name)
		case failingConclusions[strings.ToLower(strings.TrimSpace(run.Conclusion))]:
			reading.Failing = append(reading.Failing, name)
		default:
			reading.Passing = append(reading.Passing, name)
		}
	}
	return reading, nil
}

// annotations reads one check run's annotations: the files they name, each once
// and in order, and the first of them whole, as the forge wrote them.
func (g GitHub) annotations(ctx context.Context, checkRun int64) ([]string, []Annotation, error) {
	result, err := g.api(ctx, fmt.Sprintf("repos/{owner}/{repo}/check-runs/%d/annotations?per_page=%d", checkRun, maxAnnotationsRead))
	if err != nil {
		return nil, nil, err
	}
	if result.Status != execution.ProcessSucceeded {
		return nil, nil, fmt.Errorf("exit code %d: %s", result.ExitCode, g.redact(firstLine(strings.TrimSpace(result.Stderr))))
	}
	var reported []struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		Level     string `json:"annotation_level"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &reported); err != nil {
		return nil, nil, fmt.Errorf("decode the annotations of check run %d: %w", checkRun, err)
	}
	seen := map[string]bool{}
	var paths []string
	var kept []Annotation
	for _, annotation := range reported {
		path := strings.TrimSpace(annotation.Path)
		if len(kept) < maxAnnotationsKept {
			kept = append(kept, Annotation{Path: path, Line: annotation.StartLine, Level: strings.TrimSpace(annotation.Level), Message: strings.TrimSpace(annotation.Message)})
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, kept, nil
}

// RerunCheck asks the forge to run one failed check again: the Actions job
// whose check run it is, on the same head. The forge's latest reading of the
// head's checks is then the re-run's, so a job that failed in the runner rather
// than on the tree stops holding the merge once it passes.
//
// A check no Actions job ran — one a third-party app reports — has nothing the
// forge can re-run, and the forge refusing says so as an error. A token the
// forge will not let re-run jobs is refused with ErrForgeAccessRefused.
func (g GitHub) RerunCheck(ctx context.Context, checkRun int64) error {
	if checkRun <= 0 {
		return fmt.Errorf("check run %d is not a check run", checkRun)
	}
	result, err := g.apiMethod(ctx, "POST", fmt.Sprintf("repos/{owner}/{repo}/actions/jobs/%d/rerun", checkRun))
	if err != nil {
		return fmt.Errorf("ask the forge to run check %d again: %w", checkRun, err)
	}
	if result.Status != execution.ProcessSucceeded {
		return g.forgeFailure(fmt.Sprintf("ask the forge to run check %d again", checkRun), result)
	}
	return nil
}

// JobLogTail is the failing step's lines from the forge's log of the job behind
// a check run, which is what says why a check failed with no file named: GitHub
// files "Process completed with exit code 2" on .github for a red test and a
// broken runner alike, and only the log tells them apart. It is read under the
// harness's own forge access so the item the merge is withdrawn or handed back
// on carries it, because a developer run may not reach the forge at all and a
// person should not have to go and read it (yoyodyne-xko, yoyodyne-ifd.429.35).
//
// The lines are those ending at the last error the log marks — the failing
// step, rather than the clean-up steps the forge runs after it — or the log's
// last lines where it marks none.
//
// A check no Actions job ran has no log, and the forge refusing says so as an
// error. A token the forge will not let read the log is refused with
// ErrForgeAccessRefused.
func (g GitHub) JobLogTail(ctx context.Context, checkRun int64, lines int) (string, error) {
	if checkRun <= 0 {
		return "", fmt.Errorf("check run %d is not a check run", checkRun)
	}
	result, err := g.api(ctx, fmt.Sprintf("repos/{owner}/{repo}/actions/jobs/%d/logs", checkRun))
	if err != nil {
		return "", fmt.Errorf("read the forge's log of check %d: %w", checkRun, err)
	}
	if result.Status != execution.ProcessSucceeded {
		return "", g.forgeFailure(fmt.Sprintf("read the forge's log of check %d", checkRun), result)
	}
	return logTail(result.Stdout, lines), nil
}

// logErrorMarker is how an Actions log marks the line a step failed on.
const logErrorMarker = "##[error]"

// logTail is the lines of a log ending at the last error it marks, or its last
// lines where it marks none, with trailing blank lines dropped.
func logTail(log string, lines int) string {
	all := strings.Split(strings.TrimRight(log, "\n\r\t "), "\n")
	for index := len(all) - 1; index >= 0; index-- {
		if strings.Contains(all[index], logErrorMarker) {
			all = all[:index+1]
			break
		}
	}
	if lines > 0 && len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// DisableAutoMerge withdraws the merge the forge is holding for a pull
// request, so nothing lands it until it is asked for again. It is what the
// harness does before it hands a red request back or rewrites its head: a
// merge left armed would land the request the moment its checks turned green,
// which for a rewritten head is a change no reviewer has seen.
//
// Withdrawing it also takes the request out of the base branch's merge queue
// where the queue holds it. The queue consumes the auto-merge as it takes a
// request, so turning auto-merge off leaves a queued request queued, and the
// queue would land the head the harness is about to rewrite. A withdrawal whose
// dequeue fails, or that cannot learn whether the queue holds the request, is
// refused rather than reported done. The auto-merge is turned off first, so
// nothing can put the request back in the queue between the two.
//
// A request with no merge armed has nothing to withdraw, and the forge saying
// so is not a failure; nor is a forge with no merge queue, which answers that
// the queue holds nothing.
func (g GitHub) DisableAutoMerge(ctx context.Context, number int) error {
	if number <= 0 {
		return fmt.Errorf("pull request number %d is not a request", number)
	}
	scope, err := g.repoArgs(ctx)
	if err != nil {
		return err
	}
	result, err := g.exec(ctx, append([]string{"pr", "merge", strconv.Itoa(number)}, append(scope, "--disable-auto")...)...)
	if err != nil {
		return fmt.Errorf("withdraw the queued merge of pull request %d: %w", number, err)
	}
	if result.Status != execution.ProcessSucceeded {
		reason := g.redact(strings.TrimSpace(result.Stderr))
		if !nothingArmed(reason) {
			return fmt.Errorf("withdraw the queued merge of pull request %d failed with exit code %d: %s", number, result.ExitCode, reason)
		}
	}
	if err := g.dequeue(ctx, number); err != nil {
		return fmt.Errorf("withdraw the queued merge of pull request %d: %w", number, err)
	}
	return nil
}

// nothingArmed recognizes the forge saying a request has no queued merge to
// withdraw.
func nothingArmed(reason string) bool {
	normalized := normalizeAutoMerge(reason)
	return strings.Contains(normalized, "auto merge is not enabled for") ||
		strings.Contains(normalized, "auto merge is not enabled on") ||
		strings.Contains(normalized, "does not have auto merge")
}
