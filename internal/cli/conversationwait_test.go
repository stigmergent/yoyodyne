package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/chat"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

type conversationWaitBackend struct {
	requests []backendapi.RunRequest
	refusal  backendapi.RunResult
	check    func()
}

func (b *conversationWaitBackend) Run(_ context.Context, request backendapi.RunRequest) (backendapi.RunResult, error) {
	if b.check != nil {
		b.check()
	}
	b.requests = append(b.requests, request)
	result := backendapi.RunResult{SessionID: "session-after-pass", FinalText: "Answered."}
	switch len(b.requests) {
	case 1:
		result = b.refusal
	case 2:
		result.FinalText = "```yoyodyne-sweep\n" + `{"status":"complete","summary":"looked at the docket"}` + "\n```"
	}
	result.Backend = domain.BackendClaudeCode
	result.Process.Status = execution.ProcessSucceeded
	result.LastEvent = request.LastSequence + 1
	event, err := execution.NewEvent(request.RunID, result.LastEvent, time.Now(), execution.EventAgentMessage,
		"provider.test", map[string]any{"text": result.FinalText})
	if err != nil {
		return result, err
	}
	if request.EventSink != nil {
		if err := request.EventSink(event); err != nil {
			return result, err
		}
	}
	return result, nil
}

// The maintenance message waits with its hold down. A real scheduled pass
// takes that same conversation, then the original message resumes its latest
// session and event sequence. Cancellation during the wait preserves the pass.
func TestAProviderWaitLetsAScheduledPassRunAndPreservesItsTurn(t *testing.T) {
	for _, test := range []struct {
		name    string
		outage  bool
		cancel  bool
		replace bool
	}{
		{name: "usage window"},
		{name: "provider unreachable", outage: true},
		{name: "cancelled usage wait", cancel: true},
		{name: "conversation replaced during wait", replace: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			conversations, err := runstate.NewConversationStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			identity := runstate.ConversationIdentity{Agent: "development-manager", Role: domain.RoleDevelopmentManager}
			hold, err := conversations.Claim(context.Background(), identity)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Release()
			clock := &steppedClock{at: time.Now().UTC()}
			provider := &conversationWaitBackend{refusal: backendapi.RunResult{IsError: true,
				UsageLimit: &backendapi.UsageLimit{Kind: "seven_day", ResetsAt: clock.at.Add(time.Hour)}}}
			if test.outage {
				provider.refusal.UsageLimit = nil
				provider.refusal.ProviderOutage = &backendapi.ProviderOutage{Cause: domain.ProviderUnreachable, Detail: "network unavailable"}
			}
			provider.check = func() {
				other, err := conversations.TryClaim(identity)
				if other != nil {
					other.Release()
				}
				if !errors.Is(err, runstate.ErrConversationHeld) {
					t.Fatalf("provider was invoked without the conversation hold: %v", err)
				}
			}
			options := chat.Options{
				Role: identity.Role, Agent: identity.Agent, Backend: provider, Provider: domain.BackendClaudeCode,
				AccountAlias: config.DefaultAccountAlias, Store: conversations, Hold: hold, Model: "fable",
				Repository: root, ProductID: "yoyodyne", RepositoryID: "yoyodyne", Clock: clock,
				Briefing:                    chat.Briefing{Text: "A software delivery harness.", GatheredAt: clock.at},
				UsageLimitUnknownResetPause: 30 * time.Minute,
				UsageLimitPause:             chat.UsageLimitPause{Maximum: time.Hour, InProcess: time.Hour},
			}
			sweeps, err := runstate.NewSweepStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			trigger := orchestrator.Trigger{
				Tasks:  map[string]config.RecurringTask{"manager-pass": {Role: identity.Role, Every: config.Duration(time.Hour), Enabled: true, Prompt: "look at the docket"}},
				Claims: sweeps, Reports: sweeps, Clock: clock,
				Roles: roleConversation{open: func(ctx context.Context, _ domain.AgentRole, _, _ string) (*chat.Session, *runstate.ConversationHold, error) {
					passHold, err := conversations.Claim(ctx, identity)
					if err != nil {
						return nil, nil, err
					}
					passOptions := options
					passOptions.Hold = passHold
					passOptions.UsageLimitPause = chat.UsageLimitPause{}
					session, err := chat.Open(passOptions)
					return session, passHold, err
				}},
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var replacementID string
			const pending = "The scheduled pass left a result for the next turn."
			options.Sleep = func(_ context.Context, duration time.Duration) error {
				if hold.Held() {
					t.Fatal("the waiting turn still holds its conversation")
				}
				standing := readmodel.ReadStanding(context.Background(), readmodel.Sources{Conversations: conversations, Now: clock.Now})
				if len(standing.Working) != 0 || len(standing.Waiting) != 1 || !strings.Contains(standing.Render(), "asking again") {
					t.Fatalf("status does not name the released waiting turn: %s", standing.Render())
				}
				clock.at = clock.at.Add(duration)
				fired, err := trigger.Fire(context.Background())
				if err != nil || len(fired.Fired) != 1 || fired.Fired[0].Turns != 1 {
					t.Fatalf("scheduled pass during wait = %+v, %v", fired, err)
				}
				// A later process can also leave an action result, or replace the
				// whole conversation. The waiting turn must adopt the former and
				// leave the latter alone, as an interactive window does.
				otherHold, err := conversations.Claim(context.Background(), identity)
				if err != nil {
					t.Fatal(err)
				}
				defer otherHold.Release()
				if test.replace {
					fresh := options
					fresh.Hold, fresh.Fresh = otherHold, true
					other, err := chat.Open(fresh)
					if err != nil {
						t.Fatal(err)
					}
					replacementID = other.Evidence().ConversationID
				} else {
					latest, err := conversations.Load(identity)
					if err != nil {
						t.Fatal(err)
					}
					latest.PendingTrackerResults = pending
					if err := conversations.Save(latest); err != nil {
						t.Fatal(err)
					}
				}
				if test.cancel {
					cancel()
					return ctx.Err()
				}
				return nil
			}
			session, err := chat.Open(options)
			if err != nil {
				t.Fatal(err)
			}
			conversationID := session.Evidence().ConversationID
			_, err = session.Send(ctx, "Verification pass")
			if test.cancel && !errors.Is(err, context.Canceled) || test.replace && (err == nil || !strings.Contains(err.Error(), "another process started a new one")) || !test.cancel && !test.replace && err != nil {
				t.Fatalf("Send() = %v", err)
			}
			recorded, err := conversations.Load(identity)
			wantTurns := 2
			if test.cancel {
				wantTurns = 1
			}
			wantID, wantSession := conversationID, "session-after-pass"
			if test.replace {
				wantID, wantSession, wantTurns = replacementID, "", 0
			}
			if err != nil || recorded.ConversationID != wantID || recorded.Turns != wantTurns || recorded.ProviderSessionID != wantSession {
				t.Fatalf("the scheduled turn was lost: %+v, %v", recorded, err)
			}
			if test.cancel && recorded.PendingTrackerResults != pending {
				t.Fatalf("the cancelled wait overwrote a later result: %+v", recorded)
			}
			if !test.cancel && !test.replace && (len(provider.requests) != 3 || provider.requests[2].SessionID != "session-after-pass" || !strings.Contains(provider.requests[2].Prompt, pending) || !strings.Contains(provider.requests[2].Prompt, "Verification pass")) {
				t.Fatalf("the retry did not resume the pass's session: %+v", provider.requests)
			}
			if (test.cancel || test.replace) && len(provider.requests) != 2 {
				t.Fatalf("the stopped wait invoked a provider again: %+v", provider.requests)
			}
			events, err := conversations.LoadEvents(conversationID)
			if err != nil {
				t.Fatal(err)
			}
			for i, event := range events {
				if event.Sequence != uint64(i+1) {
					t.Fatalf("event %d has sequence %d", i, event.Sequence)
				}
			}
			if waits, err := conversations.WaitingTurns(); err != nil || len(waits) != 0 {
				t.Fatalf("ended wait remains on status: %+v, %v", waits, err)
			}
		})
	}
}

// Both kinds of pass spend their own bounded wait behind a live turn. A miss
// names its holder, counts no failed firing, and leaves event cursors unmoved.
func TestAScheduledPassBehindAHeldConversationIsMissedRatherThanFailed(t *testing.T) {
	for _, role := range []domain.AgentRole{domain.RoleDevelopmentManager, domain.RoleProgramManager} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			store, err := runstate.NewConversationStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			identity := runstate.ConversationIdentity{Agent: string(role), Role: role}
			hold, err := store.Claim(context.Background(), identity)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Release()
			sweeps, err := runstate.NewSweepStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			roles := roleConversation{timeout: 10 * time.Millisecond, open: func(ctx context.Context, _ domain.AgentRole, _, _ string) (*chat.Session, *runstate.ConversationHold, error) {
				_, err := store.Claim(ctx, identity)
				return nil, nil, err
			}}
			trigger := orchestrator.Trigger{Claims: sweeps, Reports: sweeps, Roles: roles}
			if role == domain.RoleProgramManager {
				trigger.Instances = map[string]config.AgentConfig{identity.Agent: {Role: role, Triggers: config.Triggers{Every: config.Duration(time.Hour)}}}
			} else {
				trigger.Tasks = map[string]config.RecurringTask{identity.Agent: {Role: role, Every: config.Duration(time.Hour), Enabled: true, Prompt: "look at the docket"}}
			}
			fired, err := trigger.Fire(context.Background())
			if err != nil || len(fired.Fired) != 1 || fired.Fired[0].NotStarted != "" {
				t.Fatalf("Fire() = %+v, %v", fired, err)
			}
			recorded, _, err := sweeps.List()
			if err != nil || len(recorded) != 1 {
				t.Fatalf("List() = %+v, %v", recorded, err)
			}
			pass := recorded[0]
			if pass.Missed == nil || pass.Missed.How != runstate.MissConversationHeld || pass.Failed || pass.NotStarted != "" || pass.Turns != 0 {
				t.Fatalf("held conversation became a failed firing: %+v", pass)
			}
			if !strings.Contains(pass.Problem, fmt.Sprintf("process %d", os.Getpid())) || !strings.Contains(pass.Problem, "context deadline exceeded") {
				t.Fatalf("miss did not name the holder and its bound: %s", pass.Problem)
			}
		})
	}
}
