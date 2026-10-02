package readmodel

import "github.com/mason-bryant/yoyodyne/internal/ownership"

// NotificationOwnership resolves a durable milestone whose producer could not
// supply its standing entry's answer. The renderer carries only the facts; the
// registry decides the owner, reason, remedy, and capability as on every surface.
func NotificationOwnership(kind ownership.NotificationKind, facts ownership.Entry) ownership.Resolution {
	facts.Kind, facts.Notification = ownership.KindNotification, kind
	return ownership.Resolve(facts)
}
