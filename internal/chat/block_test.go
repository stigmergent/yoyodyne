package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestMalformedMemoryKeepsTrackerActionsAndReachesTheNextProcess(t *testing.T) {
	t.Parallel()
	for name, block := range map[string]string{
		"JSON":            memoryBlock(`{"memories":`),
		"validation":      memoryBlock(`{"memories":[{"action":"remember","memory":"bad/name","text":"lesson"}]}`),
		"unclosed":        memoryFence + "\n{\"memories\":\n",
		"trailing opener": memoryFence + " invalid\n{}\n```\n",
		"repeated":        memoryBlock(`{"memories":[]}`) + memoryBlock(`{"memories":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			tracker := &fakeTracker{}
			var results []backendapi.RunResult
			for round := 1; round < maxTrackerRounds; round++ {
				results = append(results, backendapi.RunResult{SessionID: "session-1", FinalText: trackerReply("Reading it.", `{"action":"read","id":"yoyodyne-ifd.22"}`)})
			}
			// The malformed memory precedes the valid tracker block, including in
			// the unclosed case. The last round leaves results for a later process.
			results = append(results, backendapi.RunResult{SessionID: "session-1", FinalText: block +
				trackerReply("Closing it.", `{"action":"close","id":"yoyodyne-ifd.22","reason":"the change landed"}`)})
			provider := &fakeBackend{results: results}
			options := testOptions(t, provider)
			options.Store = newTestStore(t, root)
			options.Tracker = tracker
			session := openTestSession(t, options)
			reply, err := session.Send(context.Background(), "Settle the item.")
			if err != nil {
				t.Fatalf("Send() = %v", err)
			}
			if len(tracker.closed) != 1 || len(reply.Actions) != maxTrackerRounds || !reply.Actions[maxTrackerRounds-1].Applied {
				t.Fatalf("valid tracker actions were lost: closed=%v, actions=%+v", tracker.closed, reply.Actions)
			}
			if len(reply.BlockRefusals) != 1 || reply.BlockRefusals[0].Block != "yoyodyne-memory" || len(reply.Memories) != 0 {
				t.Fatalf("refusals=%+v, memories=%+v", reply.BlockRefusals, reply.Memories)
			}
			problem := reply.BlockRefusals[0].Problem
			payload := onlyEventPayload(t, root, session, execution.EventReplyBlockRefused)
			if !strings.Contains(payload, `"block":"yoyodyne-memory"`) {
				t.Fatalf("refusal was not recorded: %s", payload)
			}
			if !strings.Contains(reply.RefusalProblems(), problem) {
				t.Fatal("the scheduled pass would lose the refusal")
			}
			resumedProvider := &fakeBackend{results: []backendapi.RunResult{
				{SessionID: "session-1", FinalText: "I will correct that memory."},
				{SessionID: "session-1", FinalText: "Done."},
			}}
			options.Backend = resumedProvider
			options.Store = newTestStore(t, root)
			resumed := openTestSession(t, options)
			if _, err := resumed.Send(context.Background(), "Carry on."); err != nil {
				t.Fatal(err)
			}
			if prompt := resumedProvider.requests[0].Prompt; !strings.Contains(prompt, problem) || !strings.Contains(prompt, "correct only the refused block") {
				t.Fatalf("next turn lost the refusal: %s", prompt)
			}
			if _, err := resumed.Send(context.Background(), "Anything else?"); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(resumedProvider.requests[1].Prompt, problem) {
				t.Fatal("refusal was delivered twice")
			}
		})
	}
}

func TestUnauthorizedMalformedBlockStillRefusesTheWholeReply(t *testing.T) {
	t.Parallel()
	for _, role := range []domain.AgentRole{domain.RoleDeveloper, domain.RoleProductManager} {
		t.Run(string(role), func(t *testing.T) {
			block := memoryBlock(`{"memories":`)
			if role == domain.RoleProductManager {
				block = laneReportFence + "\n{}\n```\n"
			}
			tracker := &fakeTracker{}
			action := `{"action":"close","id":"yoyodyne-ifd.22","reason":"done"}`
			if role == domain.RoleDeveloper {
				action = `{"action":"read","id":"yoyodyne-ifd.22"}`
			}
			options := testOptions(t, &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: trackerReply("Looking at it.", action) + block}}})
			options.Role, options.Agent, options.Tracker = role, string(role), tracker
			reply, err := openTestSession(t, options).Send(context.Background(), "Continue.")
			var unauthorized *AuthorityError
			if !errors.As(err, &unauthorized) || len(tracker.closed) != 0 || len(tracker.shown) != 0 || len(reply.BlockRefusals) != 0 {
				t.Fatalf("authority refusal = %v, actions=%v, refusals=%v", err, tracker.closed, reply.BlockRefusals)
			}
		})
	}
}

func TestMalformedBlocksDoNotHideOtherKinds(t *testing.T) {
	t.Parallel()
	for _, fence := range replyFences {
		if fence == trackerFence {
			continue
		}
		t.Run(fence, func(t *testing.T) {
			answer := fence + "\n{invalid\n```\n" + trackerReply("Reading it.", `{"action":"read","id":"yoyodyne-ifd.22"}`)
			parsed, err := splitReply(domain.RoleProductManager, answer)
			if err != nil || len(parsed.Actions) != 1 || !parsed.Carried[fence] {
				t.Fatalf("splitReply lost the valid block: %+v, %v", parsed, err)
			}
			if len(parsed.Refusals) == 0 && parsed.LaneReportProblem == nil {
				t.Fatal("malformed block was not refused")
			}
		})
	}
}

func requireBlockRefusal(t *testing.T, reply Reply, err error, block string) string {
	t.Helper()
	if err != nil || len(reply.BlockRefusals) != 1 || reply.BlockRefusals[0].Block != block {
		t.Fatalf("Send() = %v, refusals=%+v; want only %s refused", err, reply.BlockRefusals, block)
	}
	return reply.BlockRefusals[0].Problem
}

func TestInvalidArgumentsCannotHideAnUnauthorizedTrackerAction(t *testing.T) {
	t.Parallel()
	options := testOptions(t, &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: trackerReply("Moving it.", `{"action":"reprioritize","id":"yoyodyne-ifd.22","priority":1}`) +
		memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`)}}})
	options.Role, options.Agent = domain.RoleDevelopmentManager, string(domain.RoleDevelopmentManager)
	reply, err := openTestSession(t, options).Send(context.Background(), "Look at the docket.")
	var unauthorized *AuthorityError
	if !errors.As(err, &unauthorized) || len(reply.Memories) != 0 || len(reply.BlockRefusals) != 0 {
		t.Fatalf("authority refusal = %v, memories=%v, refusals=%v", err, reply.Memories, reply.BlockRefusals)
	}
}

func TestRepeatedBlocksCannotHideUnauthorizedActions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		role       domain.AgentRole
		permitted  string
		forbidden  string
		wantReason string
	}{
		{
			name: "tracker", role: domain.RoleDevelopmentManager,
			permitted:  trackerReply("Reading it.", `{"action":"read","id":"yoyodyne-ifd.22"}`),
			forbidden:  trackerReply("Moving it.", `{"action":"reprioritize","id":"yoyodyne-ifd.22","priority":1,"reason":"move it first"}`),
			wantReason: "reprioritize",
		},
		{
			name: "artifact", role: domain.RoleProductManager,
			permitted:  documentReply("create", "v2-goals", "goals", "docs/product", "# Goals\\n\\nShip the thing."),
			forbidden:  documentReply("create", "v2-design", "design", "docs/designs", "# Design\\n\\nBuild the thing."),
			wantReason: "design",
		},
	} {
		for _, order := range []string{"forbidden first", "forbidden last", "unclosed first"} {
			t.Run(test.name+"/"+order, func(t *testing.T) {
				first, second := test.permitted, test.forbidden
				if order == "forbidden first" {
					first, second = second, first
				} else if order == "unclosed first" {
					first = strings.TrimSuffix(first, "```\n")
				}
				provider := &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: first + second +
					memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`)}}}
				options, _ := documentOptions(t, provider)
				root := t.TempDir()
				memories, err := runstate.NewMemoryStore(root, "yoyodyne")
				if err != nil {
					t.Fatal(err)
				}
				tracker := &fakeTracker{}
				options.Role, options.Agent = test.role, string(test.role)
				options.Store, options.Memories, options.Tracker = newTestStore(t, root), memories, tracker
				session := openTestSession(t, options)
				reply, err := session.Send(context.Background(), "Carry on.")
				var unauthorized *AuthorityError
				if !errors.As(err, &unauthorized) || !strings.Contains(err.Error(), test.wantReason) {
					t.Fatalf("Send() = %v, want the unauthorized %s to refuse the whole reply", err, test.name)
				}
				if len(reply.Actions) != 0 || len(tracker.shown) != 0 || len(reply.Memories) != 0 || len(reply.Writes) != 0 ||
					len(session.Writes()) != 0 || len(reply.BlockRefusals) != 0 || len(provider.requests) != 1 {
					t.Fatalf("unauthorized reply carried out other blocks: %+v", reply)
				}
				all, problems, err := memories.Memories(string(test.role))
				if err != nil || len(problems) != 0 || len(all) != 0 {
					t.Fatalf("memory store = %v, %v, %v; want no memory written", all, problems, err)
				}
			})
		}
	}
}

func TestRepeatedAuthorizedBlocksStillFailValidation(t *testing.T) {
	t.Parallel()
	for _, block := range []string{
		trackerReply("Reading it.", `{"action":"read","id":"yoyodyne-ifd.22"}`),
		documentReply("create", "v2-goals", "goals", "docs/product", "# Goals\\n\\nShip the thing."),
	} {
		parsed, err := splitReply(domain.RoleProductManager, block+block+
			memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`))
		if parsed.AuthorityProblem != nil || (err == nil && len(parsed.Refusals) == 0) ||
			len(parsed.Actions) != 0 || len(parsed.Writes) != 0 || len(parsed.Memories) != 1 {
			t.Fatalf("duplicate validation lost a block or widened authority: %+v, %v", parsed, err)
		}
	}
}

func TestAnUnclosedBlockCannotSwallowTheScheduledPassAccount(t *testing.T) {
	t.Parallel()
	account := "```yoyodyne-sweep\n" + `{"status":"complete","summary":"looked at the docket"}` + "\n```"
	parsed, err := splitReply(domain.RoleDevelopmentManager, memoryFence+"\n{invalid\n"+account)
	if err != nil || len(parsed.Refusals) != 1 || !strings.Contains(parsed.Prose, account) {
		t.Fatalf("the malformed block swallowed the pass account: %+v, %v", parsed, err)
	}
}
