package ownership

import "strings"

// Reason is why an entry is the operator's. It is required when the owner is
// the operator and empty otherwise, and the set is closed: the operator's rule
// of 2026-09-26 leaves him a change to the fundamental goals, and the acts only
// a person can physically perform, and nothing else. Adding a reason is an
// amendment to the invariant the-operator-owns-only-what-only-he-can-do, never
// a code change on its own.
type Reason string

const (
	// ReasonFundamentalIntent is a change to what the goals admit or refuse,
	// drafted by the Lead Product Manager, or an amendment to the goals that
	// does not say which kind it is.
	ReasonFundamentalIntent Reason = "fundamental-intent"
	// ReasonCredential is a credential or an external account only a person
	// holds.
	ReasonCredential Reason = "credential"
	// ReasonForgeSetting is a forge or repository setting the harness's token
	// cannot change.
	ReasonForgeSetting Reason = "forge-setting"
	// ReasonBeyondGrant is a file beyond every grant, which today means Claude
	// Code's settings files and activating a role definition.
	ReasonBeyondGrant Reason = "beyond-grant"
	// ReasonHumanGate is a step an item reserved for a person with
	// `human-gate:`.
	ReasonHumanGate Reason = "human-gate"
	// ReasonOwnHold is lifting a hold or pause the operator placed himself.
	ReasonOwnHold Reason = "own-hold"
	// ReasonDivergedHistory is saying which history is right when a target
	// branch diverged from the forge, which needs rights on the protected branch
	// that no role holds.
	ReasonDivergedHistory Reason = "diverged-history"
)

// Reasons is the closed list, whole.
func Reasons() []Reason {
	return []Reason{
		ReasonFundamentalIntent,
		ReasonCredential,
		ReasonForgeSetting,
		ReasonBeyondGrant,
		ReasonHumanGate,
		ReasonOwnHold,
		ReasonDivergedHistory,
	}
}

// Valid reports whether a reason is on the closed list. The empty reason is
// not.
func (r Reason) Valid() bool {
	for _, known := range Reasons() {
		if r == known {
			return true
		}
	}
	return false
}

// Says is the reason in the words a surface prints beside the operator's
// possessive, so every surface that says an entry is his says why.
func (r Reason) Says() string {
	switch r {
	case ReasonFundamentalIntent:
		return "a change to what the goals admit"
	case ReasonCredential:
		return "a credential only a person holds"
	case ReasonForgeSetting:
		return "a forge setting the harness's token cannot change"
	case ReasonBeyondGrant:
		return "a file beyond every grant"
	case ReasonHumanGate:
		return "a step the item reserves for a person"
	case ReasonOwnHold:
		return "a hold he placed himself"
	case ReasonDivergedHistory:
		return "a choice between diverged histories that needs rights on the protected branch"
	}
	return ""
}

// NamingForm is how an account names its reason, in the words a role's
// contract and a remedy print: the reason's token, a colon, then what the
// operator has to do. It is a constant so the role contracts can carry it
// verbatim, and a test holds it to Reasons, so a reason added to the list and
// left out of here fails.
const NamingForm = "open the account with the reason and a colon — one of fundamental-intent, credential, forge-setting, beyond-grant, human-gate, own-hold, or diverged-history — as in \"beyond-grant: add the PreToolUse hook to .claude/settings.json\""

// NamedIn is the closed-list reason a written account names for itself, and
// the empty reason where it names none. An account names one by opening with
// it and a colon — "credential: the forge token expired" — which is how a
// report's handling or a development manager's escalation says that what it
// hands the operator is his for a reason on the list rather than by default.
// Nothing else in the text is read: a reason mentioned in passing is not one
// claimed.
func NamedIn(text string) Reason {
	head, _, found := strings.Cut(strings.TrimSpace(text), ":")
	if !found {
		return ""
	}
	reason := Reason(strings.ToLower(strings.TrimSpace(head)))
	if !reason.Valid() {
		return ""
	}
	return reason
}
