package ownership

// Kind is what sort of record one waiting entry is about. The set is closed,
// for the reason the stall's reasons are: a kind nobody named is a kind no
// surface can show or act on, and the entry an operator most needs is exactly
// the one nobody thought to give a name. The read model's attention entries
// are this vocabulary (readmodel.AttentionKind is an alias of it), and the
// registry holds one rule for every kind in it: a test fails on a kind with
// none.
type Kind string

const (
	// KindAmendment is a change proposed to a document its proposer does not
	// own, which nobody has decided.
	KindAmendment Kind = "amendment"
	// KindCarriedItem is an admitted work item marked for a conversation rather
	// than a developer run.
	KindCarriedItem Kind = "conversation-carried-item"
	// KindReports is the collected report pile, once its oldest undecided report
	// has waited longer than any working cadence would leave it.
	KindReports Kind = "report"
	// KindAmendmentQueue is the queue of proposed changes, once its oldest
	// undecided proposal has waited longer than any working cadence would leave
	// it: the report pile's sibling.
	KindAmendmentQueue Kind = "amendment-queue"
	// KindOwedStep is a run that ended still owing a step.
	KindOwedStep Kind = "owed-step"
	// KindPublication is a promotion the forge has not published.
	KindPublication Kind = "publication"
	// KindDegradedService is a part of the product its supervisor has stopped
	// restarting.
	KindDegradedService Kind = "degraded-service"
	// KindFailingTask is a recurring task whose firings have failed before their
	// first turn more than once in a row.
	KindFailingTask Kind = "failing-task"
	// KindHold is one of the switches over what the harness does: the
	// operator's hold over everything, the intake hold over what it chooses for
	// itself, and the provider holding every role at once. The entry's ID says
	// which of the three.
	KindHold Kind = "hold"
	// KindDirective is a directive that pauses work and nobody has resolved.
	KindDirective Kind = "directive"
	// KindOutage is the provider answering nobody.
	KindOutage Kind = "outage"
	// KindStall is a queue nothing is pulling from while admitted work waits
	// behind that: a session sitting idle over it, no session at all, the
	// provider answering nobody, or a target branch diverged from the forge's.
	KindStall Kind = "stall"
	// KindHeldWork is the admitted work somebody has to release, counted by
	// whose move it is rather than named item by item.
	KindHeldWork Kind = "held-work"
	// KindOperatorAction is a finding raised for the operator: a report handled
	// as needing his hand, a critical report nobody has handled, a stopped run
	// the development manager escalated, or an owning role's recommendations on
	// the changes proposed to its documents. It is always named and never
	// counted into the remainder. Whose it is is the registry's to say, and it
	// is his only for a reason on the closed list.
	KindOperatorAction Kind = "operator-action"
	// KindProductDecision is the Lead Product Manager's decision that an item
	// whose run is in flight is superseded, narrowed, or to be retired, which
	// the development manager has not yet answered by stopping the run or
	// letting it finish.
	KindProductDecision Kind = "product-decision"
	// KindHumanGate is an admitted work item declaring a step only a person can
	// take, which nobody has recorded taking — or a declaration of one that
	// nothing could read.
	KindHumanGate Kind = "human-gate"
	// KindUntracedPass is a role's last pass that reported findings and left no
	// trace of them outside its account.
	KindUntracedPass Kind = "untraced-pass"
	// KindStoppage is one stopped run, as the docket and the channel weigh it
	// by who moves next. It is not an attention entry of its own — stopped runs
	// reach the attention line counted, as held work — and it is a kind here
	// because the channel's severity asks whose it is.
	KindStoppage Kind = "stoppage"
	// KindFactoryStall is the factory pulling no work and completing no passes.
	KindFactoryStall Kind = "factory-stall"
	// KindTrackerUnanswered is the tracker failing listings after retries.
	KindTrackerUnanswered Kind = "tracker-unanswered"
	// KindPassedOver is the recorded cause of a poll starting no work.
	KindPassedOver Kind = "passed-over"
)

// Kinds is every kind the registry holds a rule for, so a test that has to
// cover every kind reads it from here rather than repeating the list.
func Kinds() []Kind {
	return []Kind{
		KindAmendment,
		KindCarriedItem,
		KindReports,
		KindAmendmentQueue,
		KindOwedStep,
		KindPublication,
		KindDegradedService,
		KindFailingTask,
		KindHold,
		KindDirective,
		KindOutage,
		KindStall,
		KindHeldWork,
		KindOperatorAction,
		KindProductDecision,
		KindHumanGate,
		KindUntracedPass,
		KindStoppage,
		KindFactoryStall,
		KindTrackerUnanswered,
		KindPassedOver,
	}
}

// Valid reports whether a token is one of the kinds.
func (k Kind) Valid() bool {
	for _, known := range Kinds() {
		if k == known {
			return true
		}
	}
	return false
}

// The three switches a KindHold entry can be about, as its ID names them. None
// of the three is a record with an identifier of its own — each is one file
// under the product, present or absent — so the name of the switch is what
// identifies it.
const (
	HoldOperator = "operator"
	HoldIntake   = "intake"
	HoldCapacity = "capacity"
)

// HeldWait is which of the two waits held work is in.
type HeldWait string

const (
	// HeldAwaitingDecision is a stoppage the development manager has still to
	// decide about.
	HeldAwaitingDecision HeldWait = "decision"
	// HeldAwaitingCarryOut is a decision she recorded that the harness has
	// still to act on.
	HeldAwaitingCarryOut HeldWait = "carry-out"
)

// Finding is which of the four shapes a KindOperatorAction entry is: where the
// finding came from, which is what its rule reads to say whose it is.
type Finding string

const (
	// FindingHandling is a report its handler recorded as needing the operator.
	FindingHandling Finding = "handling"
	// FindingCriticalReport is a report filed at critical severity that nobody
	// has handled.
	FindingCriticalReport Finding = "critical-report"
	// FindingEscalation is a stopped run the development manager escalated.
	FindingEscalation Finding = "escalation"
	// FindingAmendmentBatch is an owning role's recommendations on the changes
	// proposed to its documents, one per recurring pass.
	FindingAmendmentBatch Finding = "amendment-batch"
)
