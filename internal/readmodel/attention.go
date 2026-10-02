package readmodel

// One thing waiting on a person, carried as the record it is about.
//
// Until yoyodyne-ifd.432.5 an attention entry was two sentences: what is
// waiting, and whose move it is. A sentence can be printed and nothing else — a
// surface that wants to show a proposed change in full, count the entries by
// who has to move, or act on one has nothing to open, nothing to group by, and
// nothing to name in the act. So an entry now carries the thing: its kind, from
// a closed vocabulary; the identifier of the record it is about; who moves
// next, from a second closed vocabulary; and the record itself, whole, where
// there is one — an amendment's target document, proposer, change, and reason
// among them.
//
// The two sentences are still what a terminal prints, and they are derived
// here from those fields rather than stored beside them. That is what makes
// the record and the line one thing: nothing can carry a sentence that says
// one thing over fields that say another, because the sentence is never
// written down. The JSON a script or a page reads carries both, the fields and
// the sentences computed from them at the moment of writing, so a reader that
// only wants the line still has it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/amendment"
	"github.com/mason-bryant/yoyodyne/internal/directive"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/ownership"
	"github.com/mason-bryant/yoyodyne/internal/report"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// AttentionKind is what sort of record one attention entry is about, from the
// closed vocabulary the ownership registry holds a rule for every member of.
type AttentionKind = ownership.Kind

const (
	AttentionAmendment       = ownership.KindAmendment
	AttentionCarriedItem     = ownership.KindCarriedItem
	AttentionReports         = ownership.KindReports
	AttentionAmendmentQueue  = ownership.KindAmendmentQueue
	AttentionOwedStep        = ownership.KindOwedStep
	AttentionPublication     = ownership.KindPublication
	AttentionDegradedService = ownership.KindDegradedService
	AttentionFailingTask     = ownership.KindFailingTask
	AttentionHold            = ownership.KindHold
	AttentionDirective       = ownership.KindDirective
	AttentionOutage          = ownership.KindOutage
	AttentionStall           = ownership.KindStall
	AttentionHeldWork        = ownership.KindHeldWork
	AttentionOperatorAction  = ownership.KindOperatorAction
	AttentionProductDecision = ownership.KindProductDecision
	AttentionHumanGate       = ownership.KindHumanGate
	AttentionUntracedPass    = ownership.KindUntracedPass
	// AttentionFactoryStall is the factory having pulled no work and completed
	// no recurring pass for longer than its configured limit. The moment it last
	// did either is the ID.
	AttentionFactoryStall = ownership.KindFactoryStall
	// AttentionTrackerUnanswered is the tracker failing listings after their
	// retries, since the moment the first of them failed. That moment is the ID.
	AttentionTrackerUnanswered = ownership.KindTrackerUnanswered
)

// AttentionKinds is every kind an attention entry can be, so a test that has
// to cover every kind reads it from here rather than repeating the list. It is
// the registry's kinds less the stoppage, which reaches the line counted as
// held work rather than as an entry of its own.
func AttentionKinds() []AttentionKind {
	return []AttentionKind{
		AttentionAmendment,
		AttentionCarriedItem,
		AttentionReports,
		AttentionAmendmentQueue,
		AttentionOwedStep,
		AttentionPublication,
		AttentionDegradedService,
		AttentionFailingTask,
		AttentionHold,
		AttentionDirective,
		AttentionOutage,
		AttentionStall,
		AttentionHeldWork,
		AttentionOperatorAction,
		AttentionProductDecision,
		AttentionHumanGate,
		AttentionUntracedPass,
		AttentionFactoryStall,
		AttentionTrackerUnanswered,
	}
}

// attentionKindValid reports whether a token is one of the attention kinds.
func attentionKindValid(kind AttentionKind) bool {
	for _, known := range AttentionKinds() {
		if kind == known {
			return true
		}
	}
	return false
}

// Label is the plain wording every surface uses for an attention entry.
// The machine-facing kind stays unchanged.
func (a Attention) Label() string {
	if a.Kind == AttentionOwedStep {
		if pr := a.OwedStep.queuedMerge(); pr != nil {
			if pr.Checks != nil && pr.Checks.Red() {
				return "merge stuck"
			}
			return "merge waiting"
		}
		return "run not finished"
	}
	if a.Kind == AttentionPublication {
		if p := a.Publication; p != nil && ((p.MergeDrop != nil && (p.PullRequest == nil || !p.PullRequest.MergeQueued)) || (p.PullRequest != nil && p.PullRequest.Checks != nil && p.PullRequest.Checks.Red())) {
			return "merge stuck"
		}
		return "merge waiting"
	}
	return map[AttentionKind]string{
		AttentionAmendment:         "proposed document change",
		AttentionCarriedItem:       "work in conversation",
		AttentionReports:           "reports waiting",
		AttentionAmendmentQueue:    "document changes waiting",
		AttentionDegradedService:   "service down",
		AttentionFailingTask:       "scheduled task failing",
		AttentionHold:              "work paused",
		AttentionDirective:         "direction unresolved",
		AttentionOutage:            "provider unavailable",
		AttentionStall:             "work not starting",
		AttentionHeldWork:          "work waiting",
		AttentionOperatorAction:    "person needed",
		AttentionProductDecision:   "work decision waiting",
		AttentionHumanGate:         "person's step waiting",
		AttentionUntracedPass:      "findings not recorded",
		AttentionFactoryStall:      "nothing completing",
		AttentionTrackerUnanswered: "tracker not answering",
	}[a.Kind]
}

// The three switches an AttentionHold entry can be about, as its ID names them.
const (
	HoldOperator = ownership.HoldOperator
	HoldIntake   = ownership.HoldIntake
	HoldCapacity = ownership.HoldCapacity
)

// Mover is who has to act next on one thing waiting, from the vocabulary the
// ownership registry owns. The operator's value is named only there; a surface
// here that has to tell his entries from the rest asks IsOperator.
type Mover = ownership.Mover

const (
	MoverHarness            = ownership.Harness
	MoverForge              = ownership.Forge
	MoverProvider           = ownership.Provider
	MoverNobody             = ownership.Nobody
	MoverUnnamed            = ownership.Unnamed
	MoverProductManager     = ownership.ProductManager
	MoverArchitect          = ownership.Architect
	MoverDevelopmentManager = ownership.DevelopmentManager
	MoverProgramManager     = ownership.ProgramManager
)

// MoverOf is the mover for one of the harness's roles; see ownership.MoverOf.
func MoverOf(role domain.AgentRole) Mover { return ownership.MoverOf(role) }

// Movers is the whole vocabulary, in the order a surface lists them.
func Movers() []Mover { return ownership.Movers() }

// LaneReportMovers is the part of the vocabulary a program manager's lane
// report may name as what a blocker is waiting on.
func LaneReportMovers() []Mover { return ownership.LaneReportMovers() }

// CheckLaneReportMover refuses a token that is not one of LaneReportMovers.
func CheckLaneReportMover(token string) error { return ownership.CheckLaneReportMover(token) }

// Attention is one thing waiting on somebody: what kind of thing, which one,
// whose move it is, and the record itself where there is one. The move is half
// the fact — a thread that says something is waiting without saying who on is
// the silence this whole surface exists to end — and the record is the other
// half a surface needs to show the thing or act on it.
//
// Exactly one of the record fields is set, the one the kind names; the rest
// are absent from the JSON. What and Whose are not fields: they are the two
// sentences derived from these, and the JSON carries them computed.
type Attention struct {
	Kind AttentionKind `json:"kind"`
	// ID is the identifier of the record the entry is about: an amendment's
	// id, a directive's, a run's, a work item's, a service's name, a recurring
	// task's name, or the
	// switch an AttentionHold entry names. It is empty on the two entries that
	// are about a set rather than a record — the report pile and held work —
	// and on the stall and the outage it is the stall's reason and the
	// outage's cause, which is what identifies each of those — save a diverged
	// target's stall, keyed to its branch as `diverged-target:<branch>`, since
	// two branches can be diverged at once.
	ID string `json:"id,omitempty"`
	// Mover, OwnerReason, Remedy, and Capability are the ownership registry's
	// answer for the entry (ownership.Resolve): who moves it, why the operator
	// does where he does, the one act that ends it, and what that act needs.
	// They are set by resolved and by nothing else, so no surface decides them.
	Mover       Mover            `json:"mover"`
	OwnerReason ownership.Reason `json:"owner_reason,omitempty"`
	Remedy      string           `json:"remedy"`
	Capability  string           `json:"capability,omitempty"`
	// WorkItemID is the admitted work item the entry is about, where it is
	// about one: the carried item itself, the item a run was carrying, the
	// item an amendment's proposer was working on.
	WorkItemID string `json:"work_item_id,omitempty"`

	// Amendment is the proposed change whole, on an AttentionAmendment entry:
	// the target document, its kind and owner, the proposer's role, agent,
	// run, and work item, the change, and why.
	Amendment *amendment.Proposal `json:"amendment,omitempty"`
	// Directive is the unresolved directive whole, on an AttentionDirective
	// entry.
	Directive *directive.Directive `json:"directive,omitempty"`
	// OperatorHold, IntakeHold, and CapacityHold are the switch an
	// AttentionHold entry is about, one of them set to match the ID.
	OperatorHold *runstate.OperatorHold `json:"operator_hold,omitempty"`
	IntakeHold   *runstate.IntakeHold   `json:"intake_hold,omitempty"`
	CapacityHold *CapacityHold          `json:"capacity_hold,omitempty"`
	// Outage is the provider's outage record, on an AttentionOutage entry.
	Outage *runstate.ProviderOutage `json:"outage,omitempty"`
	// Stall is the stall as the not-startable line derived it, on an
	// AttentionStall entry.
	Stall *Stall `json:"stall,omitempty"`
	// Reports is how the pile stands, on an AttentionReports entry.
	Reports *report.Pile `json:"reports,omitempty"`
	// AmendmentQueue is how the queue of proposed changes stands, on an
	// AttentionAmendmentQueue entry.
	AmendmentQueue *amendment.Queue `json:"amendment_queue,omitempty"`
	// Service is the supervisor's record of the part it left down, on an
	// AttentionDegradedService entry.
	Service *runstate.SupervisedChild `json:"service,omitempty"`
	// FailingTask is the task, its cause, and how many firings in a row, on an
	// AttentionFailingTask entry; the task is the ID.
	FailingTask *FailingTask `json:"failing_task,omitempty"`
	// OwedStep is where the run stopped, on an AttentionOwedStep entry; the
	// run is the ID and its item is WorkItemID.
	OwedStep *OwedStep `json:"owed_step,omitempty"`
	// Publication is the promotion and what the forge holds of it, on an
	// AttentionPublication entry; the run is the ID and its item is
	// WorkItemID.
	Publication *Publication `json:"publication,omitempty"`
	// HeldWork is which wait and how many items are in it, on an
	// AttentionHeldWork entry.
	HeldWork *HeldWork `json:"held_work,omitempty"`
	// Executor is the marker that hands the item to a conversation, on an
	// AttentionCarriedItem entry; the item is WorkItemID.
	Executor domain.WorkItemExecutor `json:"executor,omitempty"`
	// OperatorAction is the finding whole, on an AttentionOperatorAction entry;
	// its key is the ID.
	OperatorAction *OperatorAction `json:"operator_action,omitempty"`
	// ProductDecision is the Lead Product Manager's decision whole, on an
	// AttentionProductDecision entry; the run it is about is the ID and its item
	// is WorkItemID.
	ProductDecision *triage.ProductDecision `json:"product_decision,omitempty"`
	// HumanGate is the step the item reserves for a person, on an
	// AttentionHumanGate entry; the item is WorkItemID.
	HumanGate *HumanGateWait `json:"human_gate,omitempty"`
	// UntracedPass is the pass and the findings it left no trace of, on an
	// AttentionUntracedPass entry; the task is the ID.
	UntracedPass *UntracedPass `json:"untraced_pass,omitempty"`
	// FactoryStall is how long the factory has done nothing, its last success,
	// and what each pass failed on, on an AttentionFactoryStall entry.
	FactoryStall *FactoryStall `json:"factory_stall,omitempty"`
	// TrackerListings is how the tracker's listings stand, on an
	// AttentionTrackerUnanswered entry: since when they have failed, how many,
	// and what the latest said.
	TrackerListings *runstate.TrackerListings `json:"tracker_listings,omitempty"`

	// titles is what the tracker calls each item, set by the reading that
	// assembled the entry, so the line a person reads names every item beside
	// its title. It is not a field of the record: What and Whose are derived
	// from the fields alone, and the titled sentences are carried beside them.
	titles *WorkItemTitles
}

// CitedWhat is What with every work item it names shown beside its title.
func (a Attention) CitedWhat() string {
	return a.titles.Cite(a.What())
}

// CitedWhose is Whose with every work item it names shown beside its title,
// read after What, so an item the line has already titled is not titled twice.
func (a Attention) CitedWhose() string {
	return a.titles.CiteAfter(a.CitedWhat(), a.Whose())
}

// Named reports an entry the attention line prints by name wherever it falls
// and never counts into "and N things not named here": a finding only the
// operator can act on, and a hold the brake placed. A line that folds those
// into a remainder has told him nothing, and nothing telling him is the month
// one class of finding once waited and the two hours the brake once stood.
func (a Attention) Named() bool {
	switch a.Kind {
	case AttentionOperatorAction:
		return true
	case AttentionHold:
		return a.IntakeHold != nil && a.IntakeHold.HeldBy == runstate.IntakeHolderBrake
	}
	return false
}

// OwedStep carries the finished run's remaining cleanup or merge settlement,
// including the last recorded check reading. The run and item are on the entry.
type OwedStep struct {
	Status runstate.Status `json:"status"`
	Phase  runstate.Phase  `json:"phase,omitempty"`
	// EndedAt is when the run ended, which is when the step began to be owed.
	// It is absent on a record that names no ending.
	EndedAt                    time.Time               `json:"ended_at,omitzero"`
	PullRequest                *runstate.PullRequest   `json:"pull_request,omitempty"`
	MergeDrop                  *runstate.MergeDrop     `json:"merge_drop,omitempty"`
	TargetBranch               string                  `json:"target_branch,omitempty"`
	CleanupFailure             string                  `json:"cleanup_failure,omitempty"`
	LandingChecks              *runstate.LandingChecks `json:"landing_checks,omitempty"`
	CompletionRecordingFailure string                  `json:"completion_recording_failure,omitempty"`
}

// queuedMerge excludes an obsolete publication without discarding the run's
// independent landing or cleanup obligations or the publication's history.
func (s *OwedStep) queuedMerge() *runstate.PullRequest {
	if s == nil || s.PullRequest == nil || !s.PullRequest.MergeQueued || s.PullRequest.Superseded != "" || s.PullRequest.HandedBack != nil {
		return nil
	}
	return s.PullRequest
}

// Publication is a promotion the forge has not published: where it was
// promoted to, the branch that carries it, the pull request the forge holds
// for it where the record holds one, and the drop where the forge dropped its
// merge. Which of the four movers it waits on is read off these.
type Publication struct {
	// TargetBranch is empty where the record holds no integration to read it
	// from; the sentence says so rather than the field carrying a placeholder.
	TargetBranch string                `json:"target_branch,omitempty"`
	Branch       string                `json:"branch"`
	PullRequest  *runstate.PullRequest `json:"pull_request,omitempty"`
	MergeDrop    *runstate.MergeDrop   `json:"merge_drop,omitempty"`
	// Unarmed is a request nothing ever asked the forge to merge
	// (runstate.State.PublicationUnasked), which is the development manager's to
	// decide rather than a person's to merge by hand. One the forge has closed
	// is offered only the re-run.
	Unarmed bool `json:"unarmed,omitempty"`
	// EndedAt is when the run that promoted it ended, which is when the
	// publication was left outstanding: the run ends with it unsettled. It is
	// absent on a record that names no ending.
	EndedAt time.Time `json:"ended_at,omitzero"`
}

// HumanGateWait is one step an admitted item reserves for a person: the gate's
// name and what the person has to do, as the item's author declared them — or,
// where nothing could read the declaration, why not. Exactly one of the two
// shapes is set.
type HumanGateWait struct {
	Gate       string `json:"gate,omitempty"`
	Statement  string `json:"statement,omitempty"`
	Unreadable string `json:"unreadable,omitempty"`
}

// HeldWait is which of the two waits held work is in; see ownership.HeldWait.
type HeldWait = ownership.HeldWait

const (
	HeldAwaitingDecision = ownership.HeldAwaitingDecision
	HeldAwaitingCarryOut = ownership.HeldAwaitingCarryOut
)

// HeldWork is how many admitted items are in one of the two waits.
type HeldWork struct {
	Awaiting HeldWait `json:"awaiting"`
	Count    int      `json:"count"`
}

// What is the thing waiting, as the terminal prints it: derived from the
// entry's record, never stored.
func (a Attention) What() string {
	switch a.Kind {
	case AttentionHold:
		switch {
		case a.OperatorHold != nil:
			return fmt.Sprintf("all harness activity is held, since %s",
				a.OperatorHold.HeldAt.UTC().Format(time.RFC3339))
		case a.IntakeHold != nil:
			what := fmt.Sprintf("intake is held, since %s: %s",
				a.IntakeHold.HeldAt.UTC().Format(time.RFC3339), singleLine(intakeClause(*a.IntakeHold), maxRefusalBytes))
			// The brake's hold names the runs that tripped it, each with its item
			// and what stopped it: on 2026-09-19 the line said only that intake
			// was held, and finding which three runs had stopped it was the
			// operator's to do.
			if a.IntakeHold.Brake != nil && len(a.IntakeHold.Brake.Blocked) > 0 {
				what += "; tripped by " + singleLine(strings.Join(a.IntakeHold.Brake.Entries(), "; "), maxBrakeEntriesBytes)
			}
			return what
		case a.CapacityHold != nil:
			what := "every role is held by the provider's usage window, since " + a.CapacityHold.Since.UTC().Format(time.RFC3339)
			if !a.CapacityHold.ResetsAt.IsZero() {
				what += ", until " + a.CapacityHold.ResetsAt.UTC().Format(time.RFC3339)
			}
			return what
		}
	case AttentionDirective:
		if a.Directive != nil {
			return fmt.Sprintf("directive %s is unresolved: %s",
				a.Directive.ID, singleLine(a.Directive.Unresolved, maxRefusalBytes))
		}
	case AttentionAmendment:
		if a.Amendment != nil {
			return fmt.Sprintf("a change to %s is proposed and undecided (%s)", a.Amendment.Artifact, a.Amendment.ID)
		}
	case AttentionOwedStep:
		if step := a.OwedStep; step != nil {
			if pr := step.queuedMerge(); pr != nil {
				what := fmt.Sprintf("merge of pull request %d for %s is queued", pr.Number, a.WorkItemID)
				if pr.Merged {
					what = fmt.Sprintf("merge of pull request %d for %s needs confirmation", pr.Number, a.WorkItemID)
				}
				if pr.Checks != nil {
					what += "; " + pr.Checks.Describe(step.TargetBranch)
				}
				return what
			}
			// A dropped merge is a separate publication decision; this entry
			// describes only the run's remaining cleanup or completion.
			if step.LandingChecks != nil && !step.LandingChecks.Finished() {
				return fmt.Sprintf("landing checks for %s ended without a recorded result; their checkout needs cleanup", a.WorkItemID)
			}
			if step.Phase != runstate.PhaseCleaningUp && step.Phase != runstate.PhaseComplete {
				return fmt.Sprintf("completion of %s is not recorded; its work item needs settlement and its branch and worktree need cleanup", a.WorkItemID)
			}
			if step.CompletionRecordingFailure != "" {
				return fmt.Sprintf("completion of %s could not be recorded: %s", a.WorkItemID, step.CompletionRecordingFailure)
			}
			what := fmt.Sprintf("cleanup of the branch and worktree for %s is not finished", a.WorkItemID)
			if step.CleanupFailure != "" {
				what += ": " + step.CleanupFailure
			}
			return what
		}
	case AttentionPublication:
		if a.Publication != nil {
			target := a.Publication.TargetBranch
			if target == "" {
				target = "an unrecorded target"
			}
			if a.Publication.PullRequest == nil {
				return fmt.Sprintf("run %s promoted %s into %s and its record holds no pull request for branch %s, so nothing has asked the forge to merge it",
					a.ID, a.WorkItemID, target, a.Publication.Branch)
			}
			if a.Publication.MergeDrop != nil && !a.Publication.PullRequest.MergeQueued {
				return fmt.Sprintf("merge of pull request %d for %s was dropped by the forge: %s", a.Publication.PullRequest.Number, a.WorkItemID, a.Publication.MergeDrop.Reason)
			}
			what := fmt.Sprintf("run %s promoted %s into %s and the forge has not published it: pull request #%d %s",
				a.ID, a.WorkItemID, target, a.Publication.PullRequest.Number, a.Publication.PullRequest.URL)
			// The checks the last sweep read ride on the line, because a merge the
			// forge is holding says nothing about whether it will land.
			if checks := a.Publication.PullRequest.Checks; checks != nil {
				what += "; " + checks.Describe(a.Publication.TargetBranch)
			}
			if waiting := a.Publication.PullRequest.TargetRed; waiting != nil && !a.Publication.PullRequest.MergeQueued {
				what += "; it " + waiting.Describe()
			}
			return what
		}
	case AttentionOutage:
		if a.Outage != nil {
			return a.Outage.Says()
		}
	case AttentionReports:
		if a.Reports != nil {
			return a.Reports.Describe()
		}
	case AttentionAmendmentQueue:
		if a.AmendmentQueue != nil {
			return a.AmendmentQueue.Describe()
		}
	case AttentionStall:
		if a.Stall != nil {
			what := a.Stall.Says
			// The provider answering nobody and a diverged target already say since
			// when in their own sentences; the two session states do not, and how
			// long a queue has been unpulled is half of what makes it worth acting
			// on.
			if a.Stall.Reason != ReasonProviderAway && a.Stall.Reason != ReasonDivergedTarget && !a.Stall.Since.IsZero() {
				what += ", since " + a.Stall.Since.UTC().Format(time.RFC3339)
			}
			return what
		}
	case AttentionDegradedService:
		if a.Service != nil {
			return fmt.Sprintf("the %s service is degraded: %s", a.Service.Service, singleLine(a.Service.Reason, maxRefusalBytes))
		}
	case AttentionFailingTask:
		if a.FailingTask != nil {
			return a.FailingTask.Says()
			if a.Publication.MergeDrop != nil {
				entry.Publication.MergeDropReason = a.Publication.MergeDrop.Reason
			}
		}
	case AttentionHeldWork:
		if a.HeldWork != nil {
			counted := count(a.HeldWork.Count, "admitted item")
			if a.HeldWork.Awaiting == HeldAwaitingCarryOut {
				return fmt.Sprintf("%s %s carry-out of a decision already recorded", counted, awaits(a.HeldWork.Count))
			}
			return fmt.Sprintf("%s %s the development manager's decision", counted, awaits(a.HeldWork.Count))
		}
	case AttentionCarriedItem:
		return fmt.Sprintf("%s is admitted for %q rather than a developer run", a.WorkItemID, a.Executor)
	case AttentionOperatorAction:
		if a.OperatorAction != nil {
			return a.OperatorAction.Says()
		}
	case AttentionHumanGate:
		if a.HumanGate != nil {
			if a.HumanGate.Unreadable != "" {
				return fmt.Sprintf("%s declares a step only a person can take that nothing could read: %s",
					a.WorkItemID, singleLine(a.HumanGate.Unreadable, maxRefusalBytes))
			}
			return fmt.Sprintf("%s waits on the gate %q: %s", a.WorkItemID, a.HumanGate.Gate,
				singleLine(a.HumanGate.Statement, maxRefusalBytes))
		}
	case AttentionProductDecision:
		if a.ProductDecision != nil {
			return fmt.Sprintf("%s while run %s is in flight, decided by %s: %s",
				a.ProductDecision.Says(a.WorkItemID), a.ID, a.ProductDecision.DecidedBy, singleLine(a.ProductDecision.Reason, maxRefusalBytes))
		}
	case AttentionUntracedPass:
		if a.UntracedPass != nil {
			return a.UntracedPass.Says()
		}
	case AttentionFactoryStall:
		if a.FactoryStall != nil {
			return a.FactoryStall.Says()
		}
	case AttentionTrackerUnanswered:
		if a.TrackerListings != nil {
			// What the latest listing said is cut to a line; the record carries it
			// whole.
			said := *a.TrackerListings
			said.Latest = singleLine(said.Latest, maxRefusalBytes)
			return said.Says()
		}
	}
	// An entry whose record is missing is still said rather than printed
	// blank: a blank line on the attention line is the confident emptiness
	// this package refuses everywhere else.
	return strings.TrimSpace(string(a.Kind)+" "+a.ID) + " is waiting and its record was not carried"
}

// Whose is whose move it is and what settles it, as the terminal prints it:
// the registry's owner and remedy, and nothing a surface worked out. It opens
// with the mover's possessive on every kind, so a surface counting the line by
// Mover and a reader of the sentence agree on who has to act.
func (a Attention) Whose() string {
	return a.resolution().Whose()
}

// resolution is the registry's answer as the entry carries it.
func (a Attention) resolution() ownership.Resolution {
	return ownership.Resolution{Owner: a.Mover, Reason: a.OwnerReason, Remedy: a.Remedy, Capability: a.Capability}
}

// resolved is the entry with the registry's answer for it: the one place the
// read model asks who owns an entry, called once per entry as it is built.
func resolved(a Attention) Attention {
	answer := ownership.Resolve(a.ownershipEntry())
	a.Mover, a.OwnerReason, a.Remedy, a.Capability = answer.Owner, answer.Reason, answer.Remedy, answer.Capability
	return a
}

// ownershipEntry is the part of the entry's record the registry's rule for its
// kind reads.
func (a Attention) ownershipEntry() ownership.Entry {
	entry := ownership.Entry{Kind: a.Kind, ID: a.ID, WorkItemID: a.WorkItemID, Amendment: a.Amendment}
	switch a.Kind {
	case AttentionOwedStep:
		if step := a.OwedStep; step != nil {
			entry.Step = &runstate.State{Status: step.Status, Phase: step.Phase, PullRequest: step.queuedMerge(), LandingChecks: step.LandingChecks, CompletionRecordingFailure: step.CompletionRecordingFailure}
		}
	case AttentionHold:
		entry.Hold = a.ID
		entry.IntakeHold = a.IntakeHold
	case AttentionOutage:
		if a.Outage != nil {
			entry.OutageCause = a.Outage.Cause
		}
	case AttentionStall:
		if a.Stall != nil {
			entry.StallReason = a.Stall.Reason
			entry.OutageCause = a.Stall.OutageCause
		}
	case AttentionFailingTask:
		if a.FailingTask != nil {
			entry.FailingCause = a.FailingTask.Cause
		}
	case AttentionPublication:
		if a.Publication != nil {
			entry.Publication = &ownership.Publication{
				PullRequest:  a.Publication.PullRequest,
				MergeDropped: a.Publication.MergeDrop != nil,
				Unarmed:      a.Publication.Unarmed,
			}
			if a.Publication.MergeDrop != nil {
				entry.Publication.MergeDropReason = a.Publication.MergeDrop.Reason
			}
		}
	case AttentionHeldWork:
		if a.HeldWork != nil {
			entry.Held = a.HeldWork.Awaiting
		}
	case AttentionCarriedItem:
		entry.Role = a.Executor.Role()
	case AttentionOperatorAction:
		if a.OperatorAction != nil {
			entry.Finding = a.OperatorAction.Finding
			entry.Account = a.OperatorAction.Needs
			entry.Ends = a.OperatorAction.Ends
			entry.Role = a.OperatorAction.Role
		}
	case AttentionHumanGate:
		if a.HumanGate != nil {
			entry.Gate = a.HumanGate.Gate
			entry.GateUnreadable = a.HumanGate.Unreadable != ""
		}
	case AttentionUntracedPass:
		if a.UntracedPass != nil {
			entry.Role = a.UntracedPass.Role
		}
	}
	return entry
}

// attentionFields is the entry's fields without its methods, so the wire shape
// can embed them without inheriting the JSON methods it is implementing.
type attentionFields Attention

// attentionWire is the entry as the JSON carries it: the fields, and the two
// sentences computed from them.
type attentionWire struct {
	attentionFields
	Label string `json:"label"`
	What  string `json:"what"`
	Whose string `json:"whose"`
	// SaidWhat and SaidWhose are the two sentences as a person reads them, with
	// each work item beside its title, and absent where that changes nothing.
	// They are what the dashboard shows; What and Whose stay the derivation, so
	// a document read back is still held to its fields.
	SaidWhat  string `json:"said_what,omitempty"`
	SaidWhose string `json:"said_whose,omitempty"`
}

// MarshalJSON writes the fields and, beside them, the two sentences derived
// from them at this moment. A reader that only wants the line has it; a
// reader that wants the record has that; and neither can be handed one that
// disagrees with the other, because the sentences are never taken from
// anywhere but the fields.
func (a Attention) MarshalJSON() ([]byte, error) {
	wire := attentionWire{attentionFields: attentionFields(a), Label: a.Label(), What: a.What(), Whose: a.Whose()}
	if said := a.CitedWhat(); said != wire.What {
		wire.SaidWhat = said
	}
	if said := a.CitedWhose(); said != wire.Whose {
		wire.SaidWhose = said
	}
	return json.Marshal(wire)
}

// UnmarshalJSON reads the fields back and refuses an entry outside the shape:
// a kind or a mover that is not in its vocabulary, an unknown field, or a
// sentence that disagrees with the fields. The sentences are derived, so a
// document may leave them out; one that carries them is held to the
// derivation, because a hand-written fixture whose line says one thing over
// fields that say another is the disagreement this shape exists to make
// impossible. The vocabularies are held for the same reason: an entry whose
// mover nobody named would print as nobody's move in particular, which is the
// silence the line exists to end. Nothing in the harness stores a standing
// document and reads it back — the only readers are the dashboard's fixtures
// and a script reading `yoyo status --json` — so the refusal costs no stored
// record its readability when a sentence is reworded.
func (a *Attention) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire attentionWire
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	decoded := Attention(wire.attentionFields)
	if wire.Label != "" && wire.Label != decoded.Label() {
		return fmt.Errorf("attention entry %s: label %q disagrees with its record", decoded.ID, wire.Label)
	}
	if !attentionKindValid(decoded.Kind) {
		return fmt.Errorf("attention entry %s: its kind %q is not one of %v", decoded.ID, decoded.Kind, AttentionKinds())
	}
	if !decoded.Mover.Valid() {
		return fmt.Errorf("attention entry %s %s: its mover %q is not one of %v", decoded.Kind, decoded.ID, decoded.Mover, Movers())
	}
	// The registry's answer is derived from the record like the sentences are,
	// so a document carrying one that disagrees is refused, and one that leaves
	// the remedy out is given the registry's.
	answer := resolved(decoded)
	if decoded.Mover != answer.Mover || (decoded.Remedy != "" && decoded.Remedy != answer.Remedy) || decoded.OwnerReason != answer.OwnerReason || (decoded.Capability != "" && decoded.Capability != answer.Capability) {
		return fmt.Errorf("attention entry %s %s: its owner %q (%q) disagrees with the ownership registry, which says %q (%q)", decoded.Kind, decoded.ID, decoded.Mover, decoded.OwnerReason, answer.Mover, answer.OwnerReason)
	}
	decoded = answer
	if strings.TrimSpace(wire.What) != "" && wire.What != decoded.What() {
		return fmt.Errorf("attention entry %s %s: its what %q disagrees with its record, which says %q", decoded.Kind, decoded.ID, wire.What, decoded.What())
	}
	if strings.TrimSpace(wire.Whose) != "" && wire.Whose != decoded.Whose() {
		return fmt.Errorf("attention entry %s %s: its whose %q disagrees with its record, which says %q", decoded.Kind, decoded.ID, wire.Whose, decoded.Whose())
	}
	*a = decoded
	return nil
}

// operatorHoldAttention is the operator's hold as the attention line carries it.
func operatorHoldAttention(hold runstate.OperatorHold) Attention {
	return resolved(Attention{Kind: AttentionHold, ID: HoldOperator, OperatorHold: &hold})
}

// intakeHoldAttention is the intake hold as the attention line carries it,
// whose move it is read by the registry off the hold's own record.
func intakeHoldAttention(hold runstate.IntakeHold) Attention {
	return resolved(Attention{Kind: AttentionHold, ID: HoldIntake, IntakeHold: &hold})
}

// IntakeHoldMover is whose move the intake hold is, as the ownership registry
// resolves it, for a surface that weighs the hold by who moves it.
func IntakeHoldMover(hold runstate.IntakeHold) Mover {
	return intakeHoldAttention(hold).Mover
}

// IntakeHoldWhose is whose move the intake hold is and what settles it, as the
// attention line says it, for a surface that says the hold on its own.
func IntakeHoldWhose(hold runstate.IntakeHold) string {
	return intakeHoldAttention(hold).Whose()
}

// directiveAttention is an unresolved directive as the attention line carries
// it.
func directiveAttention(paused directive.Directive) Attention {
	return resolved(Attention{Kind: AttentionDirective, ID: paused.ID, Directive: &paused})
}

// amendmentAttention is an undecided proposal as the attention line carries
// it, carried whole so a surface can show what was proposed and why, and act
// on it by id.
func amendmentAttention(proposal amendment.Proposal) Attention {
	return resolved(Attention{
		Kind:       AttentionAmendment,
		ID:         proposal.ID,
		WorkItemID: proposal.WorkItemID,
		Amendment:  &proposal,
	})
}

// owedStepAttention is a run that ended still owing a step, as the attention
// line carries it.
func owedStepAttention(state runstate.State) Attention {
	step := &OwedStep{Status: state.Status, Phase: state.Phase, EndedAt: runEnded(state), PullRequest: state.PullRequest, MergeDrop: state.MergeDrop, CleanupFailure: state.CleanupFailure, LandingChecks: state.LandingChecks, CompletionRecordingFailure: state.CompletionRecordingFailure}
	if state.Integration != nil {
		step.TargetBranch = state.Integration.TargetBranch
	}
	return resolved(Attention{
		Kind:       AttentionOwedStep,
		ID:         state.RunID,
		WorkItemID: state.WorkItemID,
		OwedStep:   step,
	})
}

// runEnded is when a finished run ended, and zero on a record that names no
// ending.
func runEnded(state runstate.State) time.Time {
	if state.CompletedAt == nil {
		return time.Time{}
	}
	return *state.CompletedAt
}

// outageAttention is the provider answering nobody, as the attention line
// carries it.
func outageAttention(outage runstate.ProviderOutage) Attention {
	return resolved(Attention{Kind: AttentionOutage, ID: string(outage.Cause), Outage: &outage})
}

// OutageWhose is whose move the provider answering nobody is and what settles
// it, as the attention line says it, for a surface that says the outage on its
// own.
func OutageWhose(outage runstate.ProviderOutage) string {
	return outageAttention(outage).Whose()
}

// reportsAttention is a pile whose oldest undecided report has waited too
// long, as the attention line carries it.
func reportsAttention(pile report.Pile) Attention {
	return resolved(Attention{Kind: AttentionReports, Reports: &pile})
}

// amendmentQueueAttention is a queue of proposed changes whose oldest
// undecided one has waited too long, as the attention line carries it.
func amendmentQueueAttention(queue amendment.Queue) Attention {
	return resolved(Attention{Kind: AttentionAmendmentQueue, AmendmentQueue: &queue})
}

// degradedServiceAttention is a part the supervisor has left down, as the
// attention line carries it.
func degradedServiceAttention(child runstate.SupervisedChild) Attention {
	return resolved(Attention{Kind: AttentionDegradedService, ID: string(child.Service), Service: &child})
}

// heldWorkAttention is one of the two waits held work is in, with how many
// items are in it.
func heldWorkAttention(awaiting HeldWait, items int) Attention {
	return resolved(Attention{Kind: AttentionHeldWork, HeldWork: &HeldWork{Awaiting: awaiting, Count: items}})
}

// productDecisionAttention is a product decision about a run in flight nobody
// has answered, as the attention line carries it.
func productDecisionAttention(entry triage.Entry) Attention {
	decided := *entry.ProductDecision
	return resolved(Attention{
		Kind:            AttentionProductDecision,
		ID:              entry.RunID,
		WorkItemID:      entry.WorkItemID,
		ProductDecision: &decided,
	})
}

// carriedItemAttention is an admitted item marked for a conversation, as the
// attention line carries it.
func carriedItemAttention(workItemID string, executor domain.WorkItemExecutor) Attention {
	return resolved(Attention{
		Kind:       AttentionCarriedItem,
		ID:         workItemID,
		WorkItemID: workItemID,
		Executor:   executor,
	})
}

// operatorActionAttention is a finding raised for the operator, as the
// attention line carries it; whose it is is the registry's to say.
func operatorActionAttention(action OperatorAction) Attention {
	return resolved(Attention{
		Kind:           AttentionOperatorAction,
		ID:             action.Key,
		WorkItemID:     action.WorkItemID,
		OperatorAction: &action,
	})
}

// maxBrakeEntriesBytes bounds the runs a brake's hold names on the attention
// line. Three stops with a line each fit; a storm longer than that is cut, and
// the hold's own record carries the whole.
const maxBrakeEntriesBytes = 1 << 10
