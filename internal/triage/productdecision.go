package triage

// A decision the Lead Product Manager made about work that is still running.
//
// On 2026-09-27 she decided that yoyodyne-ifd.428.34, with a developer run in
// flight, was superseded by yoyodyne-ifd.398, and that the run should be stopped
// rather than merged. Nothing carried that to the development manager: a note on
// the item is read when a stoppage on it is decided and never while its run is
// still going, and stopping a run is a decision about work in flight, which is
// hers. So the operator's assistant relayed it by hand, which by the operator's
// rule of 2026-09-26 is a defect.
//
// The decision is docketed. It is the one entry about a run that has not
// stopped, and what it asks of her is the one question a run in flight raises:
// does it stop now, with its change preserved, or does it finish? Either answer
// is a triage decision she records about the run, and either closes the entry.
// What becomes of the item afterwards — retiring it, folding the preserved change
// into the superseding work — is not asked here: retiring is the Lead Product
// Manager's, and the rest is a decision about the stoppage a stop leaves.

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The three things the Lead Product Manager can decide about an item whose run
// is in flight. Each says why the run may not be worth finishing; none of them
// says whether it is, which is the development manager's to decide.
const (
	// ProductSuperseded is an item whose work another item now does. It names
	// that item.
	ProductSuperseded = "superseded"
	// ProductNarrowed is an item whose scope was cut after its run started, so
	// the run is building more than the item now asks for.
	ProductNarrowed = "narrowed"
	// ProductRetired is an item the Lead Product Manager has decided will not be
	// done. The item is not retired by the decision: an item closed under a run
	// still working on it is a closure the run then settles over, so she retires
	// it once the run has stopped or finished.
	ProductRetired = "retired"
)

// ProductDecisionVocabulary is the three decisions in the order a reader meets
// them.
func ProductDecisionVocabulary() []string {
	return []string{ProductSuperseded, ProductNarrowed, ProductRetired}
}

// MaxProductDecisionIDBytes bounds an identifier the decision carries: the item
// that supersedes the run's, and the conversation it was made in.
const MaxProductDecisionIDBytes = 256

// ProductDecision is what the Lead Product Manager decided about the item a run
// in flight was made for, as the development manager is handed it.
type ProductDecision struct {
	// Decision is one of ProductDecisionVocabulary.
	Decision string `json:"decision"`
	// Reason is her reasoning, in her words.
	Reason string `json:"reason"`
	// SupersededBy is the item doing the work instead, on a supersession.
	SupersededBy string `json:"superseded_by,omitempty"`
	// DecidedBy is who decided it, in the words the item's notes attribute it
	// with, and Conversation and Turn are where.
	DecidedBy    string `json:"decided_by"`
	Conversation string `json:"conversation,omitempty"`
	Turn         int    `json:"turn,omitempty"`
	// RunStatus and RunPhase are where the run stood. They are written as the
	// decision is docketed and read again from the run's own record every time
	// the docket is built, because what she decides between — stopping it now or
	// letting it finish — turns on how far it has got by the time she reads it.
	RunStatus string    `json:"run_status"`
	RunPhase  string    `json:"run_phase,omitempty"`
	RunReadAt time.Time `json:"run_read_at"`
	// RunInFlight is whether the run was still in flight at that reading.
	RunInFlight bool `json:"run_in_flight"`
}

// ProductDecisionKey names the event a product decision is: this decision about
// this run. The decision is part of the key so that a second, different decision
// about the same run — narrowed, and then retired — is docketed beside the first
// rather than refused as a repeat of it, and folds beneath it as one live entry.
func ProductDecisionKey(runID, decision string) string {
	return Key(ClassProductDecision, runID) + ":" + strings.TrimSpace(decision)
}

// Says is the decision in one sentence.
func (d ProductDecision) Says(item string) string {
	var said string
	switch d.Decision {
	case ProductSuperseded:
		said = fmt.Sprintf("%s is superseded by %s", item, strings.TrimSpace(d.SupersededBy))
	case ProductNarrowed:
		said = fmt.Sprintf("%s is narrowed, so its run is building more than the item now asks for", item)
	case ProductRetired:
		said = fmt.Sprintf("%s is to be retired without being done", item)
	default:
		said = fmt.Sprintf("%s is %s", item, d.Decision)
	}
	if superseded := strings.TrimSpace(d.SupersededBy); superseded != "" && d.Decision != ProductSuperseded {
		said += "; its work goes to " + superseded
	}
	return said
}

// validate reports every contract violation in the decision.
func (d ProductDecision) validate() []error {
	var problems []error
	if !slices.Contains(ProductDecisionVocabulary(), d.Decision) {
		problems = append(problems, fmt.Errorf("product_decision: %q is not one of %s", d.Decision, strings.Join(ProductDecisionVocabulary(), ", ")))
	}
	switch reason := strings.TrimSpace(d.Reason); {
	case reason == "":
		problems = append(problems, errors.New("product_decision: the reason is required, because it is what the development manager decides from"))
	case len(reason) > MaxBlockerBytes:
		problems = append(problems, fmt.Errorf("product_decision: the reason is %d bytes, limit is %d", len(reason), MaxBlockerBytes))
	}
	if d.Decision == ProductSuperseded && strings.TrimSpace(d.SupersededBy) == "" {
		problems = append(problems, errors.New("product_decision: a supersession names the item that supersedes the run's"))
	}
	if strings.TrimSpace(d.DecidedBy) == "" {
		problems = append(problems, errors.New("product_decision: who decided it is required, because a decision nobody is named for is one nobody can answer for"))
	}
	if len(d.DecidedBy) > MaxMessageBytes {
		problems = append(problems, fmt.Errorf("product_decision: who decided it is %d bytes, limit is %d", len(d.DecidedBy), MaxMessageBytes))
	}
	if len(d.SupersededBy) > MaxProductDecisionIDBytes || len(d.Conversation) > MaxProductDecisionIDBytes {
		problems = append(problems, fmt.Errorf("product_decision: an identifier it names is limited to %d bytes", MaxProductDecisionIDBytes))
	}
	if strings.TrimSpace(d.RunStatus) == "" || d.RunReadAt.IsZero() {
		problems = append(problems, errors.New("product_decision: where the run stood, and when that was read, are required, because the decision is between stopping it and letting it finish"))
	}
	return problems
}

// renderProductDecision says what the Lead Product Manager decided, where the
// run stands, and the two answers the entry asks for. It is silent on every
// entry that is not one.
func (e Entry) renderProductDecision() string {
	decided := e.ProductDecision
	if decided == nil {
		return ""
	}
	var rendered strings.Builder
	by := strings.TrimSpace(decided.DecidedBy)
	rendered.WriteString(indented("Decided about work in flight by "+by, decided.Says(e.item())+": "+strings.TrimSpace(decided.Reason)))
	stands := fmt.Sprintf("run %s is %s", e.RunID, decided.RunStatus)
	if phase := strings.TrimSpace(decided.RunPhase); phase != "" {
		stands += " in its " + phase + " phase"
	}
	stands += ", as its record read at " + decided.RunReadAt.UTC().Format(time.RFC3339)
	rendered.WriteString(indented("Where the run stands", stands))
	if decided.RunInFlight {
		fmt.Fprintf(&rendered, "      Next mover: you — decide whether run %s stops or finishes. \"stop\" asks it to stop at its next boundary with its branch and worktree preserved; \"proceed\" lets it finish and be reviewed and promoted as it would have been. Either is a triage decision naming this run, and either closes this entry. Retiring the item stays the Lead Product Manager's, once the run has stopped or finished.\n", e.RunID)
	} else {
		fmt.Fprintf(&rendered, "      Next mover: you — run %s is no longer in flight, so there is nothing left to stop; what it left is decided about as its stoppage where it stopped, and this entry is settled at the next build.\n", e.RunID)
	}
	return rendered.String()
}
