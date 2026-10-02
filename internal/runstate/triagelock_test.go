package runstate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
)

func TestRetirementDecisionReadingHoldsOffAConcurrentDecision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first, err := NewTriageStore(root, domain.ProductID("yoyodyne"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewTriageStore(root, domain.ProductID("yoyodyne"))
	if err != nil {
		t.Fatal(err)
	}
	_, release, err := first.LockCounters(context.Background(), "yoyodyne-task")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	decision := triageDecided(TriageDecisionWait, decidedRunID)
	if _, err := second.RecordDecision(ctx, "yoyodyne-task", decision, time.Now()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("decision crossed the retirement's read: %v", err)
	}
	release()
	release = nil
	counters, err := second.RecordDecision(context.Background(), "yoyodyne-task", decision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, found := counters.DecisionOf(decidedRunID); !found {
		t.Fatal("decision was not recorded after retirement released the reading")
	}
}

func TestAnUnfinishedCheckoutRestorationCannotCarryVerificationCredit(t *testing.T) {
	t.Parallel()
	state := testState(t, StatusFailed)
	state.CheckoutRestorePending = true
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	state.ChecksPassed = &ChecksPassed{Content: "old-content", At: state.UpdatedAt}
	if err := state.Validate(); err == nil || !strings.Contains(err.Error(), "verification credit cleared") {
		t.Fatalf("pending restoration with checks passed = %v", err)
	}
	state.ChecksPassed = nil
	state.Status, state.CompletedAt = StatusRunning, nil
	if err := state.Validate(); err == nil || !strings.Contains(err.Error(), "stopped run") {
		t.Fatalf("live pending restoration = %v", err)
	}
}
