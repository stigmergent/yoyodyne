package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/chat"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The hand that carries out a stop the development manager decides is wired
// into her conversation and no other, and what it writes is the request the run
// reads: the operator's stop, naming her and the decision it carries out. A run
// that has already ended is refused, since there is nothing left to stop.
func TestOnlyTheDevelopmentManagerIsWiredTheHandThatStopsARun(t *testing.T) {
	// Not parallel: the state root the components read is set here.
	stateRoot := t.TempDir()
	t.Setenv("YOYODYNE_STATE_HOME", stateRoot)
	parts, err := buildComponents(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("buildComponents() error = %v", err)
	}

	for _, role := range []domain.AgentRole{
		domain.RoleProductManager, domain.RoleArchitect, domain.RoleProgramManager, domain.RoleDeveloper, domain.RoleReviewer,
	} {
		if stops := conversationStops(parts, role); stops != nil {
			t.Fatalf("the %s was wired the hand that stops a run", role)
		}
	}
	stops := conversationStops(parts, domain.RoleDevelopmentManager)
	if stops == nil {
		t.Fatal("the development manager was wired no way to stop a run, so every stop she decides would be refused")
	}

	started := time.Now().UTC()
	running := recordedRun(t, parts.store, runstate.StatusRunning, "yoyodyne-ifd.428.34", started)
	ended := recordedRun(t, parts.store, runstate.StatusSucceeded, "yoyodyne-ifd.428.34", started)
	if err := stops.Stoppable(context.Background(), ended.RunID); err == nil || !strings.Contains(err.Error(), "not in flight") {
		t.Fatalf("Stoppable(ended) error = %v, want a run that has ended refused", err)
	}
	if err := stops.Stoppable(context.Background(), running.RunID); err != nil {
		t.Fatalf("Stoppable(running) error = %v", err)
	}

	const by = "the development manager in conversation chat-0123456789abcdef"
	if err := stops.Stop(context.Background(), chat.DecidedStop{
		RunID:       running.RunID,
		WorkItemID:  "yoyodyne-ifd.428.34",
		Reason:      "superseded (superseded by yoyodyne-ifd.398)",
		RequestedBy: by,
	}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	request, requested, err := parts.store.StopRequested(running.RunID)
	if err != nil || !requested {
		t.Fatalf("StopRequested() = %v, %v; want the request the run reads", requested, err)
	}
	if request.RequestedBy != by || request.Decision != runstate.TriageDecisionStop || request.StoppedBy() != by {
		t.Fatalf("request = %#v, want her named and the decision it carries out", request)
	}
	if !strings.Contains(request.Reason, "yoyodyne-ifd.398") {
		t.Fatalf("request reason = %q", request.Reason)
	}
}
