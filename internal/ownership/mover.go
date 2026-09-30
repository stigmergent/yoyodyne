package ownership

import (
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/domain"
)

// Mover is who has to act next on one thing waiting: the operator, one of the
// harness's roles, the harness itself, the forge, or nobody. It is the
// vocabulary a surface counts the attention line by, so it is a closed set of
// tokens rather than the possessive a sentence prints — the possessive is
// derived from it, below, and is the one wording every sentence on the line
// opens with.
//
// It is declared here rather than in the read model because the registry is
// the one place that decides a mover, and the operator's value is named
// nowhere else in the module's code: a test over every non-test file outside
// this package fails on any use of Operator. A surface that has to tell his
// entries from the rest asks IsOperator.
type Mover string

const (
	// Operator is the person the harness works for. Only a rule in this
	// package returns him, and only with a reason from the closed list.
	Operator Mover = "operator"
	// Harness is the harness acting at its next sweep or poll: held work whose
	// decision is recorded and not yet carried out, or a promotion whose record
	// holds no pull request for the next reconcile to look up.
	Harness Mover = "harness"
	// Forge is the forge merging a request it has queued.
	Forge Mover = "forge"
	// Provider is the model provider answering again. No attention entry is
	// attributed to it — a usage window lifts on the provider's clock, which is
	// nobody's move — and it is here because a program manager's lane report may
	// name it as what a blocker is waiting on, and that report's movers are this
	// vocabulary's.
	Provider Mover = "provider"
	// Nobody is a wait nobody ends: a provider's usage window lifts on the
	// provider's clock.
	Nobody Mover = "nobody"
	// Unnamed is the role a record names where the harness cannot read which:
	// a bare marker, or one it does not recognize. It is a token rather than an
	// absence so a surface counting by mover has something to count it under.
	Unnamed Mover = "unnamed-role"

	ProductManager     = Mover(domain.RoleProductManager)
	Architect          = Mover(domain.RoleArchitect)
	DevelopmentManager = Mover(domain.RoleDevelopmentManager)
	// ProgramManager is a program manager instance, on an entry about its own
	// passes: the one role whose agents are many, so the entry's record names
	// which instance.
	ProgramManager = Mover(domain.RoleProgramManager)
)

// MoverOf is the mover for one of the harness's roles, and the unnamed mover
// for a role the harness does not recognize — which includes the empty role a
// conversation marker yields when it names none.
func MoverOf(role domain.AgentRole) Mover {
	if !role.Valid() {
		return Unnamed
	}
	return Mover(role)
}

// Movers is the whole vocabulary, in the order a surface lists them: the
// operator first, because the line is called "needs a human" and his is the
// count that says whether it needs him, and his are the only entries that line
// prints — every other mover's are printed under a line naming it; then the
// roles in the hierarchy's order; then the movers that are not people.
func Movers() []Mover {
	return []Mover{
		Operator,
		ProductManager,
		Architect,
		DevelopmentManager,
		ProgramManager,
		Mover(domain.RoleDeveloper),
		Mover(domain.RoleReviewer),
		Harness,
		Forge,
		Provider,
		Nobody,
		Unnamed,
	}
}

// Valid reports whether a token is one of the movers.
func (m Mover) Valid() bool {
	for _, known := range Movers() {
		if m == known {
			return true
		}
	}
	return false
}

// IsOperator reports whether the mover is the operator. It is how a surface
// tells his entries from the rest without naming his value, which only this
// package does.
func (m Mover) IsOperator() bool { return m == Operator }

// LaneReportMovers is the part of this vocabulary a program manager's lane
// report may name as what a blocker is waiting on: the people and parts of the
// line a lane can be held up by. It is declared here, beside the vocabulary it
// narrows, so the lane report and the attention line cannot come to disagree
// about what a mover is called.
func LaneReportMovers() []Mover {
	return []Mover{
		Operator,
		ProductManager,
		DevelopmentManager,
		Architect,
		Harness,
		Forge,
		Provider,
	}
}

// CheckLaneReportMover refuses a token that is not one of LaneReportMovers. It
// is the one conversion from a lane report's stored token to this vocabulary,
// and it is what the lane report store and the lane report block are checked
// with.
func CheckLaneReportMover(token string) error {
	allowed := LaneReportMovers()
	for _, mover := range allowed {
		if Mover(token) == mover {
			return nil
		}
	}
	names := make([]string, 0, len(allowed))
	for _, mover := range allowed {
		names = append(names, string(mover))
	}
	return fmt.Errorf("waiting_on %q is not a mover a blocker may wait on; the movers are %s", token, strings.Join(names, ", "))
}

// Possessive is the mover as every sentence on the attention line opens: "the
// operator's", "the development manager's", "nobody's". It is worded once here
// so a surface grouping the line by mover and a terminal printing it name the
// same person the same way.
func (m Mover) Possessive() string {
	switch m {
	case Operator:
		return "the operator's"
	case Harness:
		return "the harness's"
	case Forge:
		return "the forge's"
	case Provider:
		return "the provider's"
	case Nobody:
		return "nobody's"
	case Unnamed:
		return "the role it names"
	default:
		return "the " + domain.AgentRole(m).Title() + "'s"
	}
}

// WaitingOn is the head the fourth line prints over the entries a mover other
// than the operator moves: "Waiting on the development manager", "Waiting on
// the harness". It names the mover rather than a person, because none of these
// movers is the operator, and the words "a person" and "a human" are kept for
// what he moves.
func (m Mover) WaitingOn() string {
	switch m {
	case Operator:
		return "Waiting on the operator"
	case Harness:
		return "Waiting on the harness"
	case Forge:
		return "Waiting on the forge"
	case Provider:
		return "Waiting on the provider"
	case Nobody:
		return "Waiting on nobody's move"
	case Unnamed:
		return "Waiting on a role the harness cannot name"
	}
	if domain.AgentRole(m).Valid() {
		return "Waiting on the " + domain.AgentRole(m).Title()
	}
	return "Waiting on " + string(m)
}
