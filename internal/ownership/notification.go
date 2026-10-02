package ownership

import (
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// notificationRule resolves a milestone through the same rule as its standing
// entry. Missing evidence gets the ordinary unclassified answer.
func notificationRule(e Entry) (Resolution, bool) {
	switch e.Notification {
	case NotificationItemAdmitted, NotificationItemDecomposed, NotificationItemAttributed, NotificationItemReprioritized:
		if e.Carried || e.Role != "" {
			return carriedItemRule(e)
		}
	case NotificationWorkHandedOff, NotificationWorkPickedUp:
		return carriedItemRule(e)
	case NotificationTrackerRefusalUnresolved:
		return owned(ProductManager, "the harness has stopped trying to have these re-issued; she settles the lost actions with the role that asked", "the role's conversation")
	case NotificationCapCrossed:
		return owned(DevelopmentManager, "the crossing is already in force; she records the decision it makes possible", "yoyo triage")
	case NotificationMergeDropped:
		e.Publication = &Publication{PullRequest: &runstate.PullRequest{}, MergeDropped: true, MergeDropReason: e.Account}
		return publicationRule(e)
	case NotificationLandingUnverified:
		return owned(Harness, "the landing checks did not run; fix what stopped the checks and verify the target", "the landing checks")
	case NotificationRunParked, NotificationLineWaiting, NotificationStallNoticed, NotificationResidentStale:
		// The kind alone names no cause, deployment authority, or participant.
		return Resolution{}, false
	case NotificationExchangeTurn:
		if !e.ExchangeAsker.Valid() || !e.ExchangeAnswerer.Valid() {
			return Resolution{}, false
		}
		return owned(MoverOf(e.ExchangeAsker), "the answer returns to their conversation; they consider it and either ask again or close the exchange", "the role's conversation")
	case NotificationBlockerRecorded:
		return heldWait(HeldAwaitingDecision)
	case NotificationRunEnded:
		if e.Stopped != nil {
			return stoppageRule(e)
		}
	case NotificationProposalRaised:
		return amendmentRule(e)
	case NotificationExchangeClosed:
		return owned(Nobody, "back to the work the exchange was holding", "")
	case NotificationDirectiveRecorded:
		if e.Unsettled {
			return directiveRule(e)
		}
		return owned(Harness, "the work carries on under it", "the watch session")
	case NotificationDirectiveRefused:
		return owned(Nobody, "nothing was recorded, so nothing about the work has changed", "")
	case NotificationIntakeHeld, NotificationIntakeEscalated, NotificationWatchBraked:
		e.Hold = HoldIntake
		return holdRule(e)
	case NotificationHoldPlaced:
		e.Hold = HoldOperator
		return holdRule(e)
	case NotificationOperatorAction:
		return operatorActionRule(e)
	case NotificationWatchIdle:
		if e.Role != "" {
			e.PassedOver = runstate.PassedOverCarriedInConversation
		} else if e.Running > 0 {
			e.PassedOver = runstate.PassedOverAlreadyInFlight
		}
		return passedOverRule(e)
	case NotificationWatchStopped:
		e.StallReason = StallNoWatchSession
		return stallRule(e)
	case NotificationWatchRedeploying:
		e.StallReason = StallRedeploying
		return stallRule(e)
	case NotificationWatchReadRetrying:
		e.StallReason = StallStoreUnreadable
		return stallRule(e)
	case NotificationProviderWindow:
		e.UsageWindow = true
		return passedOverRule(e)
	case NotificationCapacityHold:
		e.Hold = HoldCapacity
		return holdRule(e)
	case NotificationProviderOutage:
		return outageRule(e)
	case NotificationRecurringTaskFailing:
		return failingTaskRule(e)
	case NotificationLogLineSkipped:
		return owned(Harness, "nothing after the skipped line is waiting; repair the unreadable record", "the durable log reader")
	}
	answer, ok := notificationProgress[e.Notification]
	return answer, ok
}

// notificationProgress describes completed transitions and work already in flight.
// Waiting states use their standing-entry rules in notificationRule.
var notificationProgress = map[NotificationKind]Resolution{
	NotificationItemAdmitted:         {Owner: Harness, Remedy: "when this reaches the top of the queue and a run is free"},
	NotificationItemDecomposed:       {Owner: Harness, Remedy: "when this reaches the top of the queue and a run is free"},
	NotificationItemAttributed:       {Owner: Harness, Remedy: "when this reaches the top of the queue and a run is free"},
	NotificationItemReprioritized:    {Owner: Harness, Remedy: "this is where it now gets pulled from"},
	NotificationTrackerBlockRefused:  {Owner: Harness, Remedy: "then the role that asked — a turn is started for it with the refusal in it, and the actions happen only if the role issues them again"},
	NotificationWorkApproved:         {Owner: Harness, Remedy: "when this reaches the top of the queue and a run is free"},
	NotificationWorkDeclined:         {Owner: Nobody, Remedy: "nothing was created, and nothing follows"},
	NotificationWorkCarriedOut:       {Owner: Nobody, Remedy: "the item is done"},
	NotificationRunStarted:           {Owner: MoverOf(domain.RoleDeveloper), Remedy: "until the checks say otherwise"},
	NotificationChecksPassed:         {Owner: MoverOf(domain.RoleReviewer), Remedy: "a verdict on the change"},
	NotificationChecksFailed:         {Owner: MoverOf(domain.RoleDeveloper), Remedy: "another attempt at the same item"},
	NotificationPathRefused:          {Owner: MoverOf(domain.RoleDeveloper), Remedy: "another attempt at the same item, with the refused paths taken back out"},
	NotificationReviewApproved:       {Owner: Harness, Remedy: "the promotion onto the target branch"},
	NotificationReviewRepairs:        {Owner: MoverOf(domain.RoleDeveloper), Remedy: "the findings as written"},
	NotificationRaceLost:             {Owner: Harness, Remedy: "the replayed change goes through the checks and a fresh review, and lands if they pass"},
	NotificationPromoted:             {Owner: Harness, Remedy: "publishing the change where the product publishes"},
	NotificationPublished:            {Owner: Forge, Remedy: "until the request merges"},
	NotificationMergeQueued:          {Owner: Forge, Remedy: "until it settles"},
	NotificationMergeCompleted:       {Owner: Nobody, Remedy: "the item is done"},
	NotificationMergeWaitingOnTarget: {Owner: Harness, Remedy: "on the item filed for the target's red check — it takes the merge up again once that closes, and nobody has anything to decide"},
	NotificationLandingGreen:         {Owner: Nobody, Remedy: "the target branch is as green as the landing checks can say"},
	NotificationLandingRed:           {Owner: Harness, Remedy: "on the item the landing filed — it is queued at the front, and every run until it lands is cut from a red base"},
	NotificationRunContinued:         {Owner: MoverOf(domain.RoleDeveloper), Remedy: "from where the change stopped"},
	NotificationRunEnded:             {Owner: Harness, Remedy: "nothing was recorded for anybody to decide, and the item is where the run left it"},
	NotificationUsageLimitExhausted:  {Owner: Nobody, Remedy: "nothing here moves while the limit stands"},
	NotificationModelSubstituted:     {Owner: Nobody, Remedy: "the turn was served, and the model it asked for is asked for again as soon as it can be"},
	NotificationReportFiled:          {Owner: Nobody, Remedy: "the work carried on"},
	NotificationDirectiveResolved:    {Owner: Harness, Remedy: "the work this held carries on from where it stopped"},
	NotificationDirectiveCarriedOut:  {Owner: Nobody, Remedy: "what was asked for is done, and nothing was waiting on it"},
	NotificationDirectiveWithdrawn:   {Owner: Nobody, Remedy: "the directive no longer applies, and any work it was holding carries on from where it stopped"},
	NotificationQuestionHeard:        {Owner: ProductManager, Remedy: "the answer follows in this thread"},
	NotificationIntakeReleased:       {Owner: Harness, Remedy: "the backlog is being pulled from again"},
	NotificationHoldLifted:           {Owner: Harness, Remedy: "every run that stopped for the hold carries on from its own record"},
	NotificationWatchStarted:         {Owner: Harness, Remedy: "the queue is pulled from until somebody stops it"},
	NotificationWatchResumed:         {Owner: Harness, Remedy: "work is being chosen again"},
	NotificationProviderRestored:     {Owner: Nobody, Remedy: "the line carried on by itself, and nothing was released or restarted to make it"},
	NotificationClaimReleased:        {Owner: Nobody, Remedy: "the item is pullable again and will be chosen in its turn, and whatever the run that left it produced is still on its branch"},
	NotificationBundleImprovement:    {Owner: Nobody, Remedy: "the value stands as this project has it until somebody decides otherwise, and nothing will ask again"},
	NotificationBundleImprovements:   {Owner: Nobody, Remedy: "every one of them stands as this project has it until somebody decides otherwise, and nothing will ask again"},
	NotificationCatchUpDigest:        {Owner: Nobody, Remedy: "the record holds all of it, and the thread carries on from here"},
}
