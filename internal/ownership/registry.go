// Package ownership is the one place that decides who owns a waiting entry:
// the owner, why the owner is the operator where it is, the one act that ends
// the entry, and the capability that act needs.
//
// Every surface used to work out on its own who moves an entry, and a case its
// rules did not cover fell to the operator. The registry here is a table keyed
// by the entry kinds the read model names, with one rule per kind; the read
// model calls Resolve once per entry as it builds the standing, and the
// dashboard, the Slack sink, `yoyo status`, and the needs-a-human line project
// the answer it carries. None of them derives an owner.
//
// The operator is an answer only a rule here gives, and only with a reason on
// the closed list (see Reason). A record the rule for its kind cannot classify
// resolves to the Lead Product Manager with the remedy "classify this entry",
// never to him. See docs/designs/ownership-and-the-operator.md.
package ownership

import (
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/amendment"
	"github.com/mason-bryant/yoyodyne/internal/artifact"
	"github.com/mason-bryant/yoyodyne/internal/backlog"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// Resolution is the registry's answer for one entry.
type Resolution struct {
	// Owner is who moves the entry next, from the mover vocabulary.
	Owner Mover `json:"owner"`
	// Reason is why the owner is the operator, from the closed list; it is
	// empty whenever the owner is anybody else.
	Reason Reason `json:"reason,omitempty"`
	// Remedy is the one act that ends the entry, in the words the surfaces
	// print after the owner's possessive.
	Remedy string `json:"remedy"`
	// Capability is the registered action, tool, or command the remedy needs.
	Capability string `json:"capability,omitempty"`
}

// Whose is the resolution as every surface says it: the owner's possessive,
// then the remedy, then — where the owner is the operator — why it is his.
func (r Resolution) Whose() string {
	whose := r.Owner.Possessive() + " — " + r.Remedy
	if says := r.Reason.Says(); says != "" {
		whose += " (his because it is " + says + ")"
	}
	return whose
}

// Entry is the part of one waiting entry's record a rule reads: its kind, and
// the facts the rule for that kind classifies by. A caller fills the facts its
// kind's rule reads and leaves the rest zero; a rule missing the fact it needs
// cannot classify the entry, and says so by resolving to the Lead Product
// Manager rather than guessing.
type Entry struct {
	Kind Kind
	// Step is the finished run's cleanup and merge obligations.
	Step *runstate.State
	// ID and WorkItemID are the record the entry is about and the item it
	// concerns, which a remedy that names the act to type puts in the act.
	ID         string
	WorkItemID string
	// Role is the role the record names: the owner of an amendment's document,
	// the role a carried item's marker names, the role whose pass left no
	// trace, or the owning role that argued a batch of proposals.
	Role domain.AgentRole
	// Amendment is the proposal whole, on an amendment.
	Amendment *amendment.Proposal
	// Hold is which switch a hold is (HoldOperator, HoldIntake, HoldCapacity),
	// and IntakeHold the intake hold's record where that is the switch — or
	// where a stall or held group is the intake hold standing.
	Hold       string
	IntakeHold *runstate.IntakeHold
	// OutageCause is why the provider answers nobody, on an outage or on a
	// stall that is one.
	OutageCause domain.ProviderOutageCause
	// StallReason is the stall's reason, on a stall or a held group stalled.
	StallReason StallReason
	// FailingCause is why a recurring task's firings failed before their first
	// turn.
	FailingCause runstate.PreTurnCause
	// Publication is how a promotion stands on the forge.
	Publication *Publication
	// Held is which of the two waits held work is in, and HeldIn the backlog's
	// group a not-startable item is held in, where the entry is one of those
	// groups rather than the held count.
	Held   HeldWait
	HeldIn backlog.HoldKind
	// Gate is the gate an item reserves for a person, and GateUnreadable is set
	// where nothing could read the declaration.
	Gate           string
	GateUnreadable bool
	// Finding is which shape a finding raised for the operator is, Account the
	// words of whoever raised it — which may open with a closed-list reason (see
	// NamedIn) — and Ends what ends it.
	Finding Finding
	Account string
	Ends    string
	// Stopped is the stopped run a stoppage is about, Found what the repository
	// held of its change (nil where nothing looked), and AwaitingCarryOut
	// whether a decision about it is recorded and not carried out.
	Stopped          *runstate.State
	Found            *triage.Found
	AwaitingCarryOut bool
	// Recovery is the recorded act that clears a stalled target or session.
	Recovery string
}

// Publication is how a promotion stands on the forge, as its rule reads it.
type Publication struct {
	PullRequest *runstate.PullRequest
	// MergeDropped is a merge the forge dropped, and Unarmed a request nothing
	// ever asked the forge to merge.
	MergeDropped    bool
	MergeDropReason string
	Unarmed         bool
}

// Rule is the registry's rule for one kind: it reads an entry's record and
// returns its resolution, and false where it cannot classify it.
type Rule func(Entry) (Resolution, bool)

// registry is the table: one rule per kind.
var registry = map[Kind]Rule{
	KindAmendment:         amendmentRule,
	KindCarriedItem:       carriedItemRule,
	KindReports:           reportsRule,
	KindAmendmentQueue:    amendmentQueueRule,
	KindOwedStep:          owedStepRule,
	KindPublication:       publicationRule,
	KindDegradedService:   degradedServiceRule,
	KindFailingTask:       failingTaskRule,
	KindHold:              holdRule,
	KindDirective:         directiveRule,
	KindOutage:            outageRule,
	KindStall:             stallRule,
	KindHeldWork:          heldWorkRule,
	KindOperatorAction:    operatorActionRule,
	KindProductDecision:   productDecisionRule,
	KindHumanGate:         humanGateRule,
	KindUntracedPass:      untracedPassRule,
	KindStoppage:          stoppageRule,
	KindFactoryStall:      factoryStallRule,
	KindTrackerUnanswered: trackerUnansweredRule,
}

// Covers reports whether the registry holds a rule for a kind.
func Covers(kind Kind) bool {
	_, ok := registry[kind]
	return ok
}

// RemedyClassify is the remedy an entry no rule can classify carries.
const RemedyClassify = "classify this entry"

// Unclassified is the resolution of an entry no rule can classify: the Lead
// Product Manager's, to classify. It is an unowned-entry defect she files
// against the registry, and it is never the operator's.
func Unclassified() Resolution {
	return Resolution{Owner: ProductManager, Remedy: RemedyClassify, Capability: "the ownership registry"}
}

// Resolve is the owner, reason, remedy, and capability of one entry: the rule
// for its kind, or Unclassified where there is no rule or the rule cannot
// classify the record. A rule that returns the operator without a reason on
// the closed list is refused here too, as unclassifiable, so no path through
// the registry names him by default.
func Resolve(entry Entry) Resolution {
	rule, ok := registry[entry.Kind]
	if !ok {
		return Unclassified()
	}
	resolved, classified := rule(entry)
	if !classified {
		return Unclassified()
	}
	if !resolved.Owner.Valid() || (resolved.Owner.IsOperator() && !resolved.Reason.Valid()) || (!resolved.Owner.IsOperator() && resolved.Reason != "") {
		return Unclassified()
	}
	return resolved
}

func owned(owner Mover, remedy, capability string) (Resolution, bool) {
	return Resolution{Owner: owner, Remedy: remedy, Capability: capability}, true
}

func operators(reason Reason, remedy, capability string) (Resolution, bool) {
	return Resolution{Owner: Operator, Reason: reason, Remedy: remedy, Capability: capability}, true
}

// amendmentRule: the role that owns the target document, and the operator
// only for a change to the goals, which today never records whether it changes
// what they admit and is therefore read as though it does.
func amendmentRule(e Entry) (Resolution, bool) {
	if e.Amendment == nil {
		return Resolution{}, false
	}
	if e.Amendment.Kind == artifact.KindGoals {
		return operators(ReasonFundamentalIntent,
			"it is a change to the goals that does not say whether it changes what they admit, and nothing reaches the document until he decides it",
			"yoyo amendment")
	}
	if !e.Amendment.Owner.Valid() {
		return Resolution{}, false
	}
	return owned(MoverOf(e.Amendment.Owner), "nothing reaches the document until they or the operator decide it", "yoyo amendment")
}

// directiveRule: the Lead Product Manager, who ends it or carries it into a
// document or item. A directive whose settling would change what the goals
// admit reaches the operator as her drafted amendment to the goals, not as
// the directive.
func directiveRule(Entry) (Resolution, bool) {
	return owned(ProductManager,
		"she ends it or carries it into a document or an item, and the work it affects waits until `yoyo directive resolve` settles it",
		"yoyo directive resolve")
}

// reportsRule and amendmentQueueRule: the role that works the pile or queue.
func reportsRule(Entry) (Resolution, bool) {
	return owned(ProductManager,
		"reports are decided in conversation, and a pile this old says the schedule that works it is not keeping up",
		"report handling")
}

func amendmentQueueRule(Entry) (Resolution, bool) {
	return owned(ProductManager,
		"proposals are decided with `yoyo amendment`, and a queue this old says the recurring task that argues them is not keeping up, or none is enabled",
		"yoyo amendment")
}

// owedStepRule: the harness, which settles what a run left owing.
func owedStepRule(e Entry) (Resolution, bool) {
	if step := e.Step; step != nil {
		if pr := step.PullRequest; pr != nil && pr.MergeQueued {
			if pr.Checks != nil && pr.Checks.Red() {
				return queuedChecksRule(pr.Checks)
			}
			return owned(Harness, "`yoyo reconcile` confirms the forge's merge and finishes the run's cleanup once it lands", "yoyo reconcile")
		}
		if step.LandingChecks != nil && !step.LandingChecks.Finished() {
			return owned(Harness, "`yoyo reconcile` records the interrupted landing as unverified and removes its checkout", "yoyo reconcile")
		}
		if step.Phase != runstate.PhaseCleaningUp && step.Phase != runstate.PhaseComplete || step.CompletionRecordingFailure != "" {
			return owned(Harness, "`yoyo reconcile` settles the work item, finishes cleanup, and records completion", "yoyo reconcile")
		}
		return owned(Harness, "`yoyo reconcile` finishes the run's cleanup and records it", "yoyo reconcile")
	}
	return owned(Harness, "`yoyo reconcile` reports which and settles it", "yoyo reconcile")
}

func factoryStallRule(Entry) (Resolution, bool) {
	return owned(Harness, "every pass it attempts is failing, so no role is looking at anything; the critical report filed when it began names the failures, and the first pull or successful pass clears this and files the recovery", "the scheduler")
}

func trackerUnansweredRule(Entry) (Resolution, bool) {
	return owned(Harness, "each listing its thirty-second bound killed is asked again, twice, before it is given up on, a pass carries on with what it could read and names what it could not, and the first listing that answers clears this", "the tracker client")
}

// queuedChecksRule is the next action for a queued merge with failed checks.
// Both publication and cleanup entries carry the same recorded check reading.
func queuedChecksRule(checks *runstate.PullRequestChecks) (Resolution, bool) {
	var remedy string
	switch {
	case checks.AwaitingRerun():
		remedy = "the forge has not yet started the job rerun the harness requested; `yoyo reconcile` leaves the merge queued and reads the jobs again, without spending another rerun"
	case checks.FailedInTheJob() && checks.Reruns < runstate.MaxCheckReruns:
		remedy = fmt.Sprintf("`yoyo reconcile` asks the forge to run the jobs it cancelled, timed out, or could not start again (%d of %d reruns on this head), leaving the merge queued; if the forge refuses, the harness withdraws the merge to update its head or return it for repair", checks.Reruns+1, runstate.MaxCheckReruns)
	case checks.FailedInTheJob():
		remedy = "the job rerun limit on this head is spent; `yoyo reconcile` withdraws the merge to update a head behind its target or return the change to the development manager"
	default:
		remedy = "`yoyo reconcile` reads the failed checks and withdraws the merge to update its head, wait for a target fix, or return the change for repair"
	}
	return owned(Harness, remedy, "yoyo reconcile")
}

// publicationRule: the harness while it can look the request up, re-arm it,
// or wait on a red target; the forge while it holds the merge queued; and the
// development manager for a merge handed back to her — dropped, never asked
// for, or unmerged with an account of what went wrong beside it.
func publicationRule(e Entry) (Resolution, bool) {
	p := e.Publication
	if p == nil {
		return Resolution{}, false
	}
	switch {
	case p.PullRequest == nil:
		return owned(Harness, "`yoyo reconcile` looks the request up on the forge by that branch, records it, and arms its merge; a forge that holds none is said on every sweep", "yoyo reconcile")
	case p.PullRequest.MergeQueued && p.PullRequest.Checks != nil && p.PullRequest.Checks.Red():
		return queuedChecksRule(p.PullRequest.Checks)
	case p.PullRequest.MergeQueued:
		return owned(Forge, "it merges once the base branch's requirements are met, and `yoyo reconcile` settles the run when it does", "the forge's merge queue")
	case p.PullRequest.TargetRed != nil:
		return owned(Harness, "the checks fail on the target itself rather than on this change, so the failure is filed as the target's and the merge waits on "+strings.Join(p.PullRequest.TargetRed.WaitingOn(), ", ")+"; once that closes the watch re-arms it on a level head that passes, or `yoyo reconcile` brings a head the fix left behind up to date, and nothing here needs a person", "yoyo reconcile")
	case p.MergeDropped:
		return owned(DevelopmentManager, "the forge dropped the merge: "+p.MergeDropReason+"; she decides a repair, re-run, or re-arm, which the harness carries out; `yoyo reconcile` settles it once the forge records the merge", "yoyo triage")
	case p.Unarmed && p.PullRequest.Closed():
		return owned(DevelopmentManager, "nothing ever asked the forge to merge the request and the forge has closed it, so there is nothing left to arm: it is on her docket, and a re-run hands the change back for a fresh run", "yoyo triage")
	case p.Unarmed:
		return owned(DevelopmentManager, "nothing ever asked the forge to merge the request, and it is on her docket: a re-arm has the harness arm it under the same landing checks the run's merge makes, a re-run hands the change back for a fresh run, and `yoyo reconcile` settles it once the forge records the merge", "yoyo triage")
	default:
		return owned(DevelopmentManager, "the request is on the forge unmerged with an account of what went wrong beside it; she re-arms it or hands it back for a re-run, and `yoyo reconcile` settles it once the forge records the merge", "yoyo triage")
	}
}

// degradedServiceRule: the harness's supervisor, which brings the part back
// once its cause is fixed.
func degradedServiceRule(Entry) (Resolution, bool) {
	return owned(Harness, "the supervisor has stopped restarting it; fix the cause, then `yoyo stop` and `yoyo start` bring it back, or start the part by hand and the supervisor takes it back", "yoyo start")
}

// failingTaskRule: the harness, whose recurring task it is, whether it
// refused what it composed or could not open the role's conversation.
func failingTaskRule(e Entry) (Resolution, bool) {
	switch e.FailingCause {
	case "":
		return Resolution{}, false
	case runstate.PreTurnConversationUnopened:
		return owned(Harness, "nothing is asked of the role until its conversation opens; what stops it opening is in the failure, and the first firing that takes a turn clears this", "the recurring task's conversation")
	default:
		return owned(Harness, "the harness refuses what it composed for the pass, which is a defect in the harness rather than anything waiting it out will end; every firing meets the same refusal until the harness is fixed, and the first firing that takes a turn clears this", "the recurring task's composition")
	}
}

// holdRule: the operator for a hold or pause he placed; for the brake's intake
// hold — one written before the brake kept its own record included — the
// development manager while she decides, the harness while it acts on
// her decision or runs a probe, and the Lead Product Manager, the next rung,
// once it is escalated; and the harness for the provider's capacity hold.
func holdRule(e Entry) (Resolution, bool) {
	switch e.Hold {
	case HoldOperator:
		return operators(ReasonOwnHold, "nothing runs until `yoyo resume` lifts it", "yoyo resume")
	case HoldIntake:
		if e.IntakeHold == nil {
			return Resolution{}, false
		}
		hold := *e.IntakeHold
		remedy := hold.Settles()
		switch {
		case hold.HeldBy == runstate.IntakeHolderOperator:
			return operators(ReasonOwnHold, remedy, "yoyo release")
		case hold.HeldBy == runstate.IntakeHolderBrake && !hold.Braked():
			// A brake hold written before the brake kept its own record: nothing
			// works it, and it is still the brake's, so it is hers to release.
			return owned(DevelopmentManager, "nothing new is chosen until `yoyo release` lifts it", "yoyo release")
		case !hold.Braked():
			return Resolution{}, false
		}
		switch {
		case hold.Brake.Decision == runstate.BrakeDecisionRelease:
			return owned(Harness, remedy, "yoyo release")
		case hold.Brake.Escalated():
			return owned(ProductManager, remedy, "yoyo release")
		case hold.Brake.Decision == runstate.BrakeDecisionProbe, hold.Brake.Probing():
			return owned(Harness, remedy, "a probe run")
		default:
			return owned(DevelopmentManager, remedy, "the brake decision")
		}
	case HoldCapacity:
		return owned(Harness, "the window lifts on the provider's clock, and enabling failover on the agents is what would move the work onto another model before it does", "model failover")
	}
	return Resolution{}, false
}

// outageRule: nobody, since the provider answering again lifts it; and the
// operator only for a login that expired, since the credential is his.
func outageRule(e Entry) (Resolution, bool) {
	switch e.OutageCause {
	case domain.ProviderUnauthenticated:
		return operators(ReasonCredential, "log in to the provider; the harness resumes on its own once it answers, and nothing is released or restarted", "the provider login")
	case domain.ProviderUnreachable:
		return owned(Nobody, "the harness resumes on its own once the network returns and the provider answers, and nothing is released or restarted", "")
	}
	return Resolution{}, false
}

// stallRule: whose the queue's stall is, by its reason. The switches are the
// hold rule's, the provider answering nobody the outage rule's, and a target
// branch diverged from the forge's the operator's, since saying which history
// is right needs rights on the protected branch no role holds.
func stallRule(e Entry) (Resolution, bool) {
	switch e.StallReason {
	case "":
		return Resolution{}, false
	case StallOperatorHold:
		return holdRule(Entry{Kind: KindHold, Hold: HoldOperator})
	case StallIntakeHold:
		return holdRule(Entry{Kind: KindHold, Hold: HoldIntake, IntakeHold: e.IntakeHold})
	case StallProviderAway:
		if e.OutageCause == "" {
			return owned(Nobody, "the harness resumes on its own once the provider answers, and nothing is released or restarted; a login that lapsed is the operator's to renew, and the outage's own entry says whether that is the cause", "")
		}
		return outageRule(e)
	case StallDivergedTarget:
		return operators(ReasonDivergedHistory, runstate.DivergedTargetRecovery, "rights on the protected branch")
	case StallNoCapacity:
		return owned(Nobody, "a slot frees as a run in flight finishes", "")
	case StallProviderWindow:
		return owned(Nobody, "the harness asks again when the provider's usage window lifts", "")
	case StallTrackerWait:
		return owned(Nobody, "the dispatch asks the tracker again on its own, and puts the item on the development manager's docket only once the recovery window is spent", "")
	case StallStoreUnreadable:
		return owned(Harness, "the queue could not be read, and it is read again until it answers or the session gives up on it", "the watch session")
	case StallSessionIdle:
		return owned(Harness, "a queue with ready work and an idle session is a stall rather than a rest, and the session's own idle line says what it passed over and why", "the watch session")
	case StallRedeploying:
		return owned(Nobody, "the session restarts into the deployed build on its own, and the session that comes back re-adopts the runs it stopped", "")
	case StallNoWatchSession:
		return owned(Harness, "nothing pulls the queue until a watch session runs; the supervisor keeps one running once `yoyo start` has started the product, and `yoyo work --watch` starts one by hand", "yoyo start")
	case StallUnwatched:
		return owned(Nobody, "no session has ever watched this product, so items run only when somebody names them; `yoyo work --watch` starts one", "")
	}
	return Resolution{}, false
}

// heldWorkRule: the owner of the group the work is held in. The held count on
// the attention line is the development manager's where a decision is owed and
// the harness's where one is recorded and not carried out; a not-startable
// group is whoever moves what it waits on.
func heldWorkRule(e Entry) (Resolution, bool) {
	switch e.HeldIn {
	case "":
		return heldWait(e.Held)
	case backlog.HeldForAPerson:
		switch e.Held {
		case HeldAwaitingDecision:
			return owned(DevelopmentManager, "she decides what becomes of each stopped run: a repair, a re-run, a wait, a re-scope, or an escalation", "yoyo triage")
		case HeldAwaitingCarryOut:
			return owned(Harness, "the harness acts on the recorded decision — a repair, a re-run, or a re-armed merge — at its next pull", "yoyo triage")
		}
		return Resolution{}, false
	case backlog.HeldByDirective:
		return owned(ProductManager, "the Lead Product Manager ends it or carries it into a document or an item, `yoyo directive resolve` settles it, and the work it pauses is pulled", "yoyo directive resolve")
	case backlog.HeldForAGate:
		return operators(ReasonHumanGate, "the operator records each act with `yoyo gate record <name> --for <item>`, and closing an item never passes one; a declaration nothing could read waits on its author correcting it", "yoyo gate record")
	case backlog.HeldByStall:
		answer, classified := stallRule(e)
		if classified && e.Recovery != "" {
			answer.Remedy = e.Recovery
		}
		return answer, classified
	case backlog.HeldWaitingOn:
		return owned(Harness, "the harness pulls each once the work it waits on lands", "the watch session")
	case backlog.HeldByConversation:
		if !e.Role.Valid() {
			return Resolution{}, false
		}
		return owned(MoverOf(e.Role), "the role does the work in its conversation, and the harness closes each item when the role's revision lands", "the role's conversation")
	case backlog.HeldParked:
		return owned(ProductManager, "she releases each once what it was parked for is settled", "the backlog order")
	case backlog.HeldCovered:
		return owned(Harness, "the harness runs the children; nothing pulls the covering item itself", "the watch session")
	case backlog.HeldUnread:
		return owned(Nobody, "the refusal beside each item says what could not be read", "")
	}
	return Resolution{}, false
}

func heldWait(held HeldWait) (Resolution, bool) {
	switch held {
	case HeldAwaitingDecision:
		return owned(DevelopmentManager, "nothing pulls a stopped item until she decides what happens to it", "yoyo triage")
	case HeldAwaitingCarryOut:
		return owned(Harness, "the decision is made, and what is outstanding is the harness acting on it", "yoyo triage")
	}
	return Resolution{}, false
}

// operatorActionRule: a finding raised for the operator is his only where the
// account that raised it names a reason from the closed list. Otherwise a
// handling and a critical report are the Lead Product Manager's, who handles
// reports; an escalation is hers too, as the rung above the development
// manager who raised it; and an owning role's batch of recommendations is
// that role's, whose decision it is.
func operatorActionRule(e Entry) (Resolution, bool) {
	ends := ""
	if strings.TrimSpace(e.Ends) != "" {
		ends = "; " + e.Ends
	}
	switch e.Finding {
	case FindingHandling, FindingEscalation:
		if reason := NamedIn(e.Account); reason != "" {
			return operators(reason, "only a person can act on this"+ends, string(reason))
		}
		if e.Finding == FindingEscalation {
			return owned(ProductManager, "the development manager escalated the stopped run and named no reason it is the operator's, so it is the Lead Product Manager's, the role above her: she settles it, sends it to the role that owns it, or, where it is one only a person can act on, has it escalated again to "+NamingForm+ends, "report handling")
		}
		return owned(ProductManager, "the handling named the operator without a reason on the closed list: she settles it, sends it to the role that owns it, or, where only a person can act on it, handles it again with \"needs\": \"operator\" and a reason that does "+NamingForm+" — only such a handling reaches him"+ends, "report handling")
	case FindingCriticalReport:
		return owned(ProductManager, "she handles the critical report: settles it, or sends it to the role whose remedy it is"+ends, "report handling")
	case FindingAmendmentBatch:
		if !e.Role.Valid() {
			return Resolution{}, false
		}
		return owned(MoverOf(e.Role), "the owning role has argued them, and `yoyo amendment` records each decision, which needs the operator's hand until owning roles decide amendments themselves (yoyodyne-ifd.437.14)"+ends, "yoyo amendment")
	}
	return Resolution{}, false
}

// productDecisionRule: the development manager, who answers the Lead Product
// Manager's decision by stopping the run or letting it finish.
func productDecisionRule(Entry) (Resolution, bool) {
	return owned(DevelopmentManager, "it is on her docket: \"stop\" stops the run with its change preserved and \"proceed\" lets it finish, and the run goes on until she records one", "yoyo triage")
}

// humanGateRule: the operator, since the item reserved the step for a person;
// a declaration nothing could read is the Lead Product Manager's, who admitted
// the item and corrects its declaration.
func humanGateRule(e Entry) (Resolution, bool) {
	if e.GateUnreadable {
		return owned(ProductManager, "no act records this one; the item's author has to correct the declaration on it before anything pulls it", "the work item's declaration")
	}
	if e.Gate == "" {
		return Resolution{}, false
	}
	return operators(ReasonHumanGate, fmt.Sprintf("nothing machinery does passes it, closing an item included; `yoyo gate record %s --for %s` is the act", e.Gate, e.WorkItemID), "yoyo gate record")
}

// untracedPassRule: the role whose pass it was.
func untracedPassRule(e Entry) (Resolution, bool) {
	if !e.Role.Valid() {
		return Resolution{}, false
	}
	return owned(MoverOf(e.Role), "its next pass is told which findings they were, and a pass of the task that takes a turn clears this; nothing here needs a person", "the recurring task")
}

// carriedItemRule: the role its executor marker names.
func carriedItemRule(e Entry) (Resolution, bool) {
	if !e.Role.Valid() {
		return Resolution{}, false
	}
	return owned(MoverOf(e.Role), "in conversation; no run will ever be started for it", "the role's conversation")
}

// stoppageRule: the development manager, save where the harness carries the
// stoppage on with nobody deciding anything — an approved change the
// environment stopped whose branch is still there, a decision recorded and
// not carried out, a check stage the harness continues, or a first
// silent-stream stall.
func stoppageRule(e Entry) (Resolution, bool) {
	if e.Stopped == nil {
		return Resolution{}, false
	}
	run := *e.Stopped
	switch {
	case run.IntegrationStop != nil && triage.IntegrationResumable(e.Found, run.BranchRemoved),
		e.AwaitingCarryOut,
		run.HarnessContinuesCheckStage(),
		run.HarnessContinuesStall():
		return owned(Harness, "the harness carries it on at its next pull, with nobody deciding anything", "the watch session")
	}
	return owned(DevelopmentManager, "nothing pulls the stopped item until she decides what becomes of the run", "yoyo triage")
}
