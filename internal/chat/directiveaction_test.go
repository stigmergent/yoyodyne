package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/directive"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// The Lead Product Manager ends two directives from her own tracker block: a
// question recorded as one is withdrawn, and a standing rule written into a
// document is resolved into it. Each is recorded on the directive as her act, in
// her name, and neither applies afterwards.
func TestTheProductManagerEndsADirectiveFromHerConversation(t *testing.T) {
	t.Parallel()

	directives := &fakeDirectives{}
	question, err := directives.Record(context.Background(), DirectiveRequest{Kind: directive.KindOperational, Text: "Is this still running?"})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	rule, err := directives.Record(context.Background(), DirectiveRequest{Kind: directive.KindOperational, Text: "Notes are append-only."})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	// A standing rule carried out earlier keeps that outcome when it is resolved.
	if _, err := directives.CarryOut(context.Background(), rule.ID, "became yoyodyne-ifd.122"); err != nil {
		t.Fatalf("CarryOut() error = %v", err)
	}
	root := t.TempDir()
	options := testOptions(t, &fakeBackend{results: []backendapi.RunResult{
		{SessionID: "session-1", FinalText: trackerReply("Ending both.",
			`{"action":"directive","directive":"`+question.ID[:14]+`","decision":"withdraw","reason":"a question, not a direction"}`,
			`{"action":"directive","directive":"`+rule.ID+`","decision":"resolve","became":"docs/product/operating-rules.md","reason":"written into the operating rules"}`)},
		{SessionID: "session-1", FinalText: "Done."},
	}})
	options.Store = newTestStore(t, root)
	options.Tracker = &fakeTracker{}
	options.Directives = directives
	session := openTestSession(t, options)

	reply, err := session.Send(context.Background(), "Retire the question and resolve the rule.")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(reply.Actions) != 2 || !reply.Actions[0].Applied || !reply.Actions[1].Applied {
		t.Fatalf("actions = %#v, want both applied", reply.Actions)
	}
	if !strings.Contains(reply.Actions[0].Summary, "withdrew directive "+question.ID) {
		t.Fatalf("withdrawal summary = %q", reply.Actions[0].Summary)
	}
	if !strings.Contains(reply.Actions[1].Summary, "into docs/product/operating-rules.md") {
		t.Fatalf("resolution summary = %q", reply.Actions[1].Summary)
	}

	recorded, _ := directives.List(context.Background())
	for _, candidate := range recorded {
		if candidate.InForce() {
			t.Errorf("directive %s is still in force: %#v", candidate.ID, candidate)
		}
		switch candidate.ID {
		case question.ID:
			if candidate.WithdrawnRole != domain.RoleProductManager || !strings.Contains(candidate.WithdrawnBy, "Product Manager, from conversation") ||
				candidate.Withdrawal != "a question, not a direction" {
				t.Errorf("withdrawn = %#v, want her act, in her name, with her reason", candidate)
			}
		case rule.ID:
			if candidate.Became != "docs/product/operating-rules.md" || candidate.BecameRole != domain.RoleProductManager ||
				!strings.Contains(candidate.BecameBy, "Product Manager, from conversation") || candidate.BecameReason != "written into the operating rules" {
				t.Errorf("resolved = %#v, want what it became, her act, in her name, with her reason", candidate)
			}
			if candidate.Resolution != "became yoyodyne-ifd.122" {
				t.Errorf("resolution = %q, want the earlier outcome kept", candidate.Resolution)
			}
		}
	}
	if payload := onlyEventPayload(t, root, session, execution.EventDirectiveWithdrawn); !strings.Contains(payload, `"by":"product-manager"`) {
		t.Fatalf("withdrawal event = %s, want who withdrew it", payload)
	}
	if payload := onlyEventPayload(t, root, session, execution.EventDirectiveResolved); !strings.Contains(payload, "operating-rules.md") {
		t.Fatalf("resolution event = %s, want what it became", payload)
	}
}

// A directive that has already ended is refused, and the refusal says how it
// ended rather than recording a second ending over the first.
func TestEndingADirectiveAlreadyEndedIsRefused(t *testing.T) {
	t.Parallel()

	directives := &fakeDirectives{}
	gone, err := directives.Record(context.Background(), DirectiveRequest{Kind: directive.KindOperational, Text: "Started?"})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if _, err := directives.Withdraw(context.Background(), gone.ID, "Mason, at a terminal", "", "a question"); err != nil {
		t.Fatalf("Withdraw() error = %v", err)
	}
	options := testOptions(t, &fakeBackend{results: []backendapi.RunResult{
		{SessionID: "session-1", FinalText: trackerReply("Ending it.",
			`{"action":"directive","directive":"`+gone.ID+`","decision":"resolve","became":"docs/product/operating-rules.md","reason":"r"}`,
			`{"action":"directive","directive":"`+gone.ID+`","decision":"withdraw","reason":"r"}`)},
		{SessionID: "session-1", FinalText: "It had ended."},
	}})
	options.Tracker = &fakeTracker{}
	options.Directives = directives
	session := openTestSession(t, options)

	reply, err := session.Send(context.Background(), "End it.")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(reply.Actions) != 2 {
		t.Fatalf("actions = %#v, want two", reply.Actions)
	}
	for _, action := range reply.Actions {
		if action.Applied || !strings.Contains(action.Failure, "was withdrawn at") {
			t.Fatalf("action = %#v, want it refused naming the withdrawal", action)
		}
	}
	recorded, _ := directives.Find(context.Background(), gone.ID)
	if recorded.WithdrawnBy != "Mason, at a terminal" || recorded.ResolvedInto() {
		t.Fatalf("recorded = %#v, want the first ending left as it was", recorded)
	}
}

// The action is the Lead Product Manager's alone.
func TestOnlyTheProductManagerEndsADirective(t *testing.T) {
	t.Parallel()

	action := TrackerAction{Action: actionDirective, Directive: "directive-05d6", Decision: directiveWithdraw, Reason: "why"}
	for _, role := range domain.Roles() {
		authority, _ := AuthorityFor(role)
		if got, want := authority.MayAct(actionDirective), role == domain.RoleProductManager; got != want {
			t.Errorf("the %s may end a directive = %v, want %v", role, got, want)
		}
		if role == domain.RoleProductManager {
			continue
		}
		session := &Session{options: Options{Lane: "reliability"}}
		session.state.Role = role
		var refusal *AuthorityError
		if err := session.authorize(parsedReply{Actions: []TrackerAction{action}}); !errors.As(err, &refusal) {
			t.Errorf("authorize() for the %s = %v, want an authority refusal", role, err)
		}
	}
}

func TestADirectiveActionIsCheckedBeforeAnythingRuns(t *testing.T) {
	t.Parallel()

	valid := []TrackerAction{
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveWithdraw, Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveResolve, Became: "docs/product/operating-rules.md", Reason: "r"},
	}
	for _, good := range valid {
		if err := good.Validate(); err != nil {
			t.Fatalf("Validate(%#v) = %v, want it accepted", good, err)
		}
	}
	for _, bad := range []TrackerAction{
		{Action: actionDirective, Decision: directiveWithdraw, Reason: "r"},
		{Action: actionDirective, Directive: "yoyodyne-ifd.1", Decision: directiveWithdraw, Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: "retire", Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveResolve, Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveResolve, Became: "a\nb", Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveWithdraw, Became: "docs/x.md", Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveWithdraw},
		{Action: actionDirective, ID: "yoyodyne-ifd.1", Directive: "directive-05d6", Decision: directiveWithdraw, Reason: "r"},
		{Action: actionDirective, Directive: "directive-05d6", Decision: directiveWithdraw, Reason: strings.Repeat("r", directive.MaxWithdrawalBytes+1)},
		{Action: actionUpdate, ID: "yoyodyne-ifd.1", Note: "n", Became: "docs/x.md", Reason: "r"},
	} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("Validate(%#v) = nil, want it refused", bad)
		}
	}
}
