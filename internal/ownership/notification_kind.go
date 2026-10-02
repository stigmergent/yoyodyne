package ownership

// NotificationKind is a durable milestone whose next move the registry resolves.
// notify.Kind aliases it so event classification and ownership share one vocabulary.
type NotificationKind string

const (
	NotificationItemAdmitted             NotificationKind = "backlog.admitted"
	NotificationItemDecomposed           NotificationKind = "backlog.decomposed"
	NotificationItemAttributed           NotificationKind = "backlog.attributed"
	NotificationItemReprioritized        NotificationKind = "backlog.reprioritized"
	NotificationTrackerBlockRefused      NotificationKind = "tracker.block-refused"
	NotificationTrackerRefusalUnresolved NotificationKind = "tracker.refusal-unresolved"
	NotificationWorkApproved             NotificationKind = "proposed-work.approved"
	NotificationWorkDeclined             NotificationKind = "proposed-work.declined"
	NotificationWorkHandedOff            NotificationKind = "work.handed-off"
	NotificationWorkPickedUp             NotificationKind = "work.picked-up"
	NotificationWorkCarriedOut           NotificationKind = "work.carried-out"
	NotificationCapCrossed               NotificationKind = "cap.crossed"
	NotificationRunStarted               NotificationKind = "run.started"
	NotificationChecksPassed             NotificationKind = "checks.passed"
	NotificationChecksFailed             NotificationKind = "checks.failed"
	NotificationPathRefused              NotificationKind = "paths.refused"
	NotificationReviewApproved           NotificationKind = "review.approved"
	NotificationReviewRepairs            NotificationKind = "review.repairs"
	NotificationRaceLost                 NotificationKind = "promotion.race-lost"
	NotificationPromoted                 NotificationKind = "promotion.made"
	NotificationPublished                NotificationKind = "publication.opened"
	NotificationMergeQueued              NotificationKind = "merge.queued"
	NotificationMergeCompleted           NotificationKind = "merge.completed"
	NotificationMergeDropped             NotificationKind = "merge.dropped"
	NotificationMergeWaitingOnTarget     NotificationKind = "merge.waiting-on-target"
	NotificationLandingGreen             NotificationKind = "landing.green"
	NotificationLandingRed               NotificationKind = "landing.red"
	NotificationLandingUnverified        NotificationKind = "landing.unverified"
	NotificationRunParked                NotificationKind = "run.parked"
	NotificationRunContinued             NotificationKind = "run.continued"
	NotificationBlockerRecorded          NotificationKind = "blocker.recorded"
	NotificationRunEnded                 NotificationKind = "run.ended"
	NotificationUsageLimitExhausted      NotificationKind = "usage-limit.exhausted"
	NotificationModelSubstituted         NotificationKind = "model.substituted"
	NotificationReportFiled              NotificationKind = "report.filed"
	NotificationProposalRaised           NotificationKind = "proposal.raised"
	NotificationExchangeTurn             NotificationKind = "exchange.turn"
	NotificationExchangeClosed           NotificationKind = "exchange.closed"
	NotificationDirectiveRecorded        NotificationKind = "directive.recorded"
	NotificationDirectiveResolved        NotificationKind = "directive.resolved"
	NotificationDirectiveCarriedOut      NotificationKind = "directive.carried-out"
	NotificationDirectiveRefused         NotificationKind = "directive.refused"
	NotificationDirectiveWithdrawn       NotificationKind = "directive.withdrawn"
	NotificationQuestionHeard            NotificationKind = "question.heard"
	NotificationIntakeHeld               NotificationKind = "intake.held"
	NotificationIntakeReleased           NotificationKind = "intake.released"
	NotificationIntakeEscalated          NotificationKind = "intake.escalated"
	NotificationHoldPlaced               NotificationKind = "hold.placed"
	NotificationHoldLifted               NotificationKind = "hold.lifted"
	NotificationOperatorAction           NotificationKind = "operator-action.recorded"
	NotificationWatchStarted             NotificationKind = "watch.started"
	NotificationWatchIdle                NotificationKind = "watch.idle"
	NotificationWatchBraked              NotificationKind = "watch.braked"
	NotificationWatchResumed             NotificationKind = "watch.resumed"
	NotificationWatchStopped             NotificationKind = "watch.stopped"
	NotificationWatchRedeploying         NotificationKind = "watch.redeploying"
	NotificationWatchReadRetrying        NotificationKind = "watch.read-retrying"
	NotificationLineWaiting              NotificationKind = "line.waiting"
	NotificationResidentStale            NotificationKind = "resident.stale"
	NotificationStallNoticed             NotificationKind = "stall.noticed"
	NotificationProviderWindow           NotificationKind = "provider.window"
	NotificationCapacityHold             NotificationKind = "capacity.hold"
	NotificationProviderOutage           NotificationKind = "provider.outage"
	NotificationProviderRestored         NotificationKind = "provider.restored"
	NotificationRecurringTaskFailing     NotificationKind = "recurring.failing"
	NotificationClaimReleased            NotificationKind = "claim.released"
	NotificationBundleImprovement        NotificationKind = "bundle.improvement"
	NotificationBundleImprovements       NotificationKind = "bundle.improvements"
	NotificationCatchUpDigest            NotificationKind = "catch-up.digest"
	NotificationLogLineSkipped           NotificationKind = "log.line-skipped"
)

// NotificationKinds is the complete milestone vocabulary.
func NotificationKinds() []NotificationKind {
	return []NotificationKind{
		NotificationItemAdmitted,
		NotificationItemDecomposed,
		NotificationItemAttributed,
		NotificationItemReprioritized,
		NotificationTrackerBlockRefused,
		NotificationTrackerRefusalUnresolved,
		NotificationWorkApproved,
		NotificationWorkDeclined,
		NotificationWorkHandedOff,
		NotificationWorkPickedUp,
		NotificationWorkCarriedOut,
		NotificationCapCrossed,
		NotificationRunStarted,
		NotificationChecksPassed,
		NotificationChecksFailed,
		NotificationPathRefused,
		NotificationReviewApproved,
		NotificationReviewRepairs,
		NotificationRaceLost,
		NotificationPromoted,
		NotificationPublished,
		NotificationMergeQueued,
		NotificationMergeCompleted,
		NotificationMergeDropped,
		NotificationMergeWaitingOnTarget,
		NotificationLandingGreen,
		NotificationLandingRed,
		NotificationLandingUnverified,
		NotificationRunParked,
		NotificationRunContinued,
		NotificationBlockerRecorded,
		NotificationRunEnded,
		NotificationUsageLimitExhausted,
		NotificationModelSubstituted,
		NotificationReportFiled,
		NotificationProposalRaised,
		NotificationExchangeTurn,
		NotificationExchangeClosed,
		NotificationDirectiveRecorded,
		NotificationDirectiveResolved,
		NotificationDirectiveCarriedOut,
		NotificationDirectiveRefused,
		NotificationDirectiveWithdrawn,
		NotificationQuestionHeard,
		NotificationIntakeHeld,
		NotificationIntakeReleased,
		NotificationIntakeEscalated,
		NotificationHoldPlaced,
		NotificationHoldLifted,
		NotificationOperatorAction,
		NotificationWatchStarted,
		NotificationWatchIdle,
		NotificationWatchBraked,
		NotificationWatchResumed,
		NotificationWatchStopped,
		NotificationWatchRedeploying,
		NotificationWatchReadRetrying,
		NotificationLineWaiting,
		NotificationResidentStale,
		NotificationStallNoticed,
		NotificationProviderWindow,
		NotificationCapacityHold,
		NotificationProviderOutage,
		NotificationProviderRestored,
		NotificationRecurringTaskFailing,
		NotificationClaimReleased,
		NotificationBundleImprovement,
		NotificationBundleImprovements,
		NotificationCatchUpDigest,
		NotificationLogLineSkipped,
	}
}

// Valid reports whether this is a registered milestone.
func (k NotificationKind) Valid() bool {
	for _, known := range NotificationKinds() {
		if k == known {
			return true
		}
	}
	return false
}
