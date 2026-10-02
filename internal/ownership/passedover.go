package ownership

import "github.com/mason-bryant/yoyodyne/internal/runstate"

// passedOverRule reads the last idle poll's recorded cause, without reading a
// newer queue or deciding an owner on the surface that projects the poll.
func passedOverRule(e Entry) (Resolution, bool) {
	switch {
	case e.Unreadable:
		return stallRule(Entry{StallReason: StallStoreUnreadable})
	case e.UsageWindow:
		return owned(Nobody, "the window lifts on the provider's clock, and the queue is read again when it does", "")
	}
	switch e.PassedOver {
	case runstate.PassedOverCarriedInConversation:
		if !e.Role.Valid() {
			return Resolution{}, false
		}
		return owned(MoverOf(e.Role), "in conversation; the work this poll passed over is carried there, and no run will ever start it", "the role's conversation")
	case runstate.PassedOverParked:
		return owned(ProductManager, "a parked item is passed over at every pull until it is released", "the backlog order")
	case runstate.PassedOverHeldForAPerson:
		return owned(DevelopmentManager, "nothing pulls a stopped item until her decision about it is made and carried out; this old class does not record which step remains", "yoyo triage")
	case runstate.PassedOverAwaitingDecision:
		return owned(DevelopmentManager, "nothing pulls a stopped item until she decides what happens to it", "yoyo triage")
	case runstate.PassedOverAwaitingCarryOut:
		return owned(Harness, "the decisions are recorded, and what is outstanding is the harness acting on them", "yoyo triage")
	case runstate.PassedOverWaitingOnAPerson:
		return operators(ReasonHumanGate, "the item declares a human gate, and `yoyo gate record` is the only thing that passes it", "yoyo gate record")
	case runstate.PassedOverWaitingOnOtherWork:
		return owned(Nobody, "the work they wait on lands or does not, and the queue is read again either way", "")
	case runstate.PassedOverAlreadyTried:
		return owned(Nobody, "the session tries them again once they have cooled", "")
	case runstate.PassedOverAlreadyInFlight:
		return owned(Nobody, "the runs carrying them finish, and the queue is read again as each of them does", "")
	case runstate.PassedOverCoveredByChildren:
		return owned(Nobody, "the children are the work, and what covers them closes as they land", "")
	case runstate.PassedOverPausedByDirective:
		return owned(ProductManager, "she ends the directive or carries it into a document or an item, and the work stays paused until it is resolved", "yoyo directive resolve")
	case runstate.PassedOverSequencedBehindWork:
		return owned(Nobody, "each is pulled at the first pull where the run it would have raced has ended", "")
	case runstate.PassedOverPrerequisiteUnmet:
		return owned(DevelopmentManager, "the item asks for something the tree does not have, and it is docketed rather than dispatched", "yoyo triage")
	case runstate.PassedOverLeftForAnotherSlot:
		return owned(Nobody, "a developer slot with no preference takes them in the Lead Product Manager's order, and a preferring slot falls back to them once its label's work is exhausted", "")
	case runstate.PassedOverWaitingOnUsageWindow:
		return owned(Nobody, "the window lifts on the provider's clock, and the session pulls each of them again once its reset has passed", "")
	}
	return Resolution{}, false
}
