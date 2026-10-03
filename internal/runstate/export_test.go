package runstate

import "time"

// SetPromotionWaitForTest shortens the real queue's bound for the external
// pipeline test. It exists only in test binaries, like the per-store clock
// setting the queue's own tests use.
func SetPromotionWaitForTest(store *Store, wait time.Duration) {
	store.promotionWait = wait
}
