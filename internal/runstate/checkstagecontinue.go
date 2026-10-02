package runstate

// Check and stage timeouts share bounded continuations. Load can explain a
// slow check, but cannot prove the change did not cause it to hang.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxCheckStageContinuations bounds how many times the harness continues one
// run at its checks after the stage bound stopped it. It is small on purpose:
// a stage the bound stops again and again at a load the harness judged low
// enough to try is a suite that does not fit its bound, which is a decision
// about the gate or the bound, not something another try settles. Past it the
// stoppage is the development manager's, as every such stoppage was before.
const MaxCheckStageContinuations = 2

// CheckTimeAllowance is reserved before execution, not after it. Its limit
// is pinned once at twice the stage's maximum load-scaled bound. Reservations
// survive a crash even when no completion was recorded.
type CheckTimeAllowance struct {
	LimitSeconds    int64 `json:"limit_seconds"`
	ReservedSeconds int64 `json:"reserved_seconds"`
	// StoppedAtExhaustion records a refused stage separately from how the last
	// executed stage ended. A failed or interrupted stage has no timeout flag.
	StoppedAtExhaustion bool `json:"stopped_at_exhaustion,omitempty"`
}

// CheckContinuationCount includes both automatic and decided continuations.
func (s State) CheckContinuationCount() int {
	count := len(s.CheckStageContinuations)
	for _, continuation := range s.RepairContinuations {
		if continuation.CheckStage {
			count++
		}
	}
	return count
}

// CheckAllowanceSays is the durable upper bound on time already allowed,
// including stages interrupted before they could record what they spent.
func (s State) CheckAllowanceSays() string {
	if s.CheckTimeAllowance == nil {
		return "no cumulative time allowance was recorded"
	}
	return fmt.Sprintf("%s of %s cumulative check time allowance reserved",
		time.Duration(s.CheckTimeAllowance.ReservedSeconds)*time.Second,
		time.Duration(s.CheckTimeAllowance.LimitSeconds)*time.Second)
}

func (s State) CheckAllowanceExhausted() bool {
	return s.CheckTimeAllowance != nil && s.CheckTimeAllowance.ReservedSeconds >= s.CheckTimeAllowance.LimitSeconds
}

// CheckStageContinuation is one continuation of this run at its checks by the
// harness, after the stage bound stopped it.
type CheckStageContinuation struct {
	// Stage preserves the tested revision, load and limits before a new stage
	// replaces the current one. Older records omit it.
	Stage           *CheckStage `json:"stage,omitempty"`
	ReservedSeconds int64       `json:"reserved_seconds,omitempty"`
	// Command is the check the bound stopped the stage during.
	Command     string    `json:"command,omitempty"`
	ContinuedAt time.Time `json:"continued_at"`
	// Reason is the harness's own account of why the run is going again.
	Reason string `json:"reason"`
	// SupersededFailure is what the stopped run ended on, in the words it was
	// recorded in.
	SupersededFailure string `json:"superseded_failure,omitempty"`
}

// Validate reports every contract violation in the record at once.
func (c CheckStageContinuation) Validate() error {
	var problems []error
	if c.Stage != nil {
		if err := c.Stage.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("stage: %w", err))
		}
	}
	if c.ContinuedAt.IsZero() {
		problems = append(problems, errors.New("continued_at is required"))
	}
	if strings.TrimSpace(c.Reason) == "" {
		problems = append(problems, errors.New("the reason the harness continued this run is required"))
	}
	if len(c.Reason) > MaxSelectionReasonBytes {
		problems = append(problems, fmt.Errorf("reason is %d bytes, which exceeds the %d byte bound", len(c.Reason), MaxSelectionReasonBytes))
	}
	if len(c.SupersededFailure) > MaxBlockerBytes {
		problems = append(problems, fmt.Errorf("superseded_failure is %d bytes, which exceeds the %d byte bound", len(c.SupersededFailure), MaxBlockerBytes))
	}
	return errors.Join(problems...)
}

// validateCheckStageContinuations reports every contract violation in the
// record's account of the harness having continued it at its checks.
func (s State) validateCheckStageContinuations() []error {
	var problems []error
	if allowance := s.CheckTimeAllowance; allowance != nil {
		if allowance.LimitSeconds <= 0 || allowance.ReservedSeconds < 0 || allowance.ReservedSeconds > allowance.LimitSeconds {
			problems = append(problems, errors.New("check time allowance must have a positive limit and reservations within that limit"))
		}
		if allowance.StoppedAtExhaustion && !s.CheckAllowanceExhausted() {
			problems = append(problems, errors.New("a stop at check time allowance exhaustion requires the allowance to be exhausted"))
		}
	}
	if len(s.CheckStageContinuations) > MaxCheckStageContinuations {
		problems = append(problems, fmt.Errorf("%d check stage continuations are recorded, which exceeds the bound of %d", len(s.CheckStageContinuations), MaxCheckStageContinuations))
	}
	for index, continuation := range s.CheckStageContinuations {
		if err := continuation.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("check_stage_continuations[%d]: %w", index, err))
		}
	}
	if len(s.CheckStageContinuationRefused) > MaxBlockerBytes {
		problems = append(problems, fmt.Errorf("check_stage_continuation_refused is %d bytes, which exceeds the %d byte bound", len(s.CheckStageContinuationRefused), MaxBlockerBytes))
	}
	return problems
}

// StoppedAtStageBound reports a run that ended because its check stage reached
// a check, stage or cumulative time limit: timed out, at its checks, with the
// record saying the bound is what ended it, and nothing promoted. Whatever else
// the record carries, nothing judged the change this round.
func (s State) StoppedAtStageBound() bool {
	return s.Status == StatusTimedOut &&
		s.Phase == PhaseChecking &&
		((s.CheckStage != nil && (s.CheckStage.StoppedAtBound || s.CheckStage.StoppedAtCheckBound)) ||
			(s.CheckTimeAllowance != nil && s.CheckTimeAllowance.StoppedAtExhaustion)) &&
		s.Integration == nil
}

// HarnessContinuesCheckStage reports a run the stage bound stopped that the
// harness will continue at its checks by itself: its branch and worktree are
// still there, the developer attempt that made the change is on the record,
// the harness has not already continued it MaxCheckStageContinuations times,
// and no earlier continuation was refused for something only a person can
// settle. Whether it may go now — a free slot, a load below the threshold, the
// operator's switches — is the moment's to answer rather than the record's.
func (s State) HarnessContinuesCheckStage() bool {
	if !s.StoppedAtStageBound() {
		return false
	}
	if s.WorktreePath == "" || s.Branch == "" || s.BaseCommit == "" || s.TargetBranch == "" {
		return false
	}
	if s.WorktreeRemoved || s.BranchRemoved || strings.TrimSpace(s.ProviderSessionID) == "" {
		return false
	}
	if strings.TrimSpace(s.CheckStageContinuationRefused) != "" {
		return false
	}
	return s.CheckContinuationCount() < MaxCheckStageContinuations && !s.CheckAllowanceExhausted()
}

// CheckStageLoadThreshold is the condition the harness waits for before it
// continues a stage the bound stopped, in words. It is stated once here so the
// docket, the channel, the item, and the configuration guide say one thing.
const CheckStageLoadThreshold = "the machine's one-minute load average below its number of cores"

// CheckStageStopSays is what every surface says about a run the stage bound
// stopped: that the check did not finish, its cause is unresolved, and what happens next.
// It is empty for every other run.
func (s State) CheckStageStopSays() string {
	if !s.StoppedAtStageBound() {
		return ""
	}
	stopped := "the check did not finish within its time limit; load may have contributed, but its cause remains unresolved and nothing was judged"
	stopped += "; " + s.CheckAllowanceSays()
	if s.HarnessContinuesCheckStage() {
		return fmt.Sprintf(
			"%s; the harness continues it itself, re-running the checks on the change the run already has, on the same branch and worktree, at the next pull with a developer slot free and %s — no developer is invoked and no review round, repair grant, or re-run is spent (continuation %d of %d)",
			stopped, CheckStageLoadThreshold, s.CheckContinuationCount()+1, MaxCheckStageContinuations)
	}
	if refused := strings.TrimSpace(s.CheckStageContinuationRefused); refused != "" {
		return fmt.Sprintf("%s; the harness's continuation of it was refused — %s — so what happens to it next is the development manager's decision", stopped, refused)
	}
	if s.CheckContinuationCount() >= MaxCheckStageContinuations || s.CheckAllowanceExhausted() {
		return fmt.Sprintf("%s; automatic continuation stopped after %d continuations because its count or cumulative time allowance is exhausted; the branch, worktree and developer session are preserved, and what happens to it next is the development manager's decision", stopped, s.CheckContinuationCount())
	}
	return stopped + "; its branch or worktree is gone, so the harness cannot continue it, and what happens to it next is the development manager's decision"
}
