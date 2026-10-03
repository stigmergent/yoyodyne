package chat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/artifact"
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

func TestMalformedBlocksCannotHideUnauthorizedActions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		fence      string
		role       domain.AgentRole
		permitted  string
		forbidden  string
		wantReason string
	}{
		{
			name: "tracker", role: domain.RoleDevelopmentManager,
			fence:      trackerFence,
			permitted:  trackerReply("Reading it.", `{"action":"read","id":"yoyodyne-ifd.22"}`),
			forbidden:  trackerReply("Moving it.", `{"action":"reprioritize","id":"yoyodyne-ifd.22","priority":1,"reason":"move it first"}`),
			wantReason: "reprioritize",
		},
		{
			name: "artifact", role: domain.RoleProductManager,
			fence:      artifact.WriteFence,
			permitted:  documentReply("create", "v2-goals", "goals", "docs/product", "# Goals\\n\\nShip the thing."),
			forbidden:  documentReply("create", "v2-design", "design", "docs/designs", "# Design\\n\\nBuild the thing."),
			wantReason: "design",
		},
	} {
		for _, order := range []string{"forbidden first", "forbidden last", "unclosed first", "unclosed forbidden", "trailing opener", "trailing closer", "inline payload"} {
			t.Run(test.name+"/"+order, func(t *testing.T) {
				first, second := test.permitted, test.forbidden
				switch order {
				case "forbidden first":
					first, second = second, first
				case "unclosed first":
					first = strings.TrimSuffix(first, "```\n")
				case "unclosed forbidden":
					first, second = strings.TrimSuffix(test.forbidden, "```\n"), ""
				case "trailing opener":
					first, second = strings.Replace(test.forbidden, test.fence, test.fence+" invalid", 1), ""
				case "trailing closer":
					first, second = strings.TrimSuffix(test.forbidden, "```\n")+"``` invalid\n", ""
				case "inline payload":
					first, second = strings.Replace(test.forbidden, test.fence+"\n", test.fence+" ", 1), ""
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

func TestInvalidFieldsCannotHideUnauthorizedActions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, fence, collection, forbidden, invalid, wantReason string
		role                                                    domain.AgentRole
	}{
		{
			name: "tracker action", fence: trackerFence, collection: "actions", role: domain.RoleDevelopmentManager,
			forbidden: `{"action":"reprioritize"}`, invalid: `{"action":5}`, wantReason: "reprioritize",
		},
		{
			name: "artifact action", fence: artifact.WriteFence, collection: "documents", role: domain.RoleProductManager,
			forbidden: `{"action":"create","kind":"design"}`, invalid: `{"action":5,"kind":"goals"}`, wantReason: "design",
		},
		{
			name: "artifact kind", fence: artifact.WriteFence, collection: "documents", role: domain.RoleProductManager,
			forbidden: `{"action":"create","kind":"design"}`, invalid: `{"action":"create","kind":5}`, wantReason: "design",
		},
	} {
		payloads := map[string]string{
			"invalid sibling first": `{"` + test.collection + `":[` + test.invalid + `,` + test.forbidden + `]}`,
			"invalid sibling last":  `{"` + test.collection + `":[` + test.forbidden + `,` + test.invalid + `]}`,
			"unclosed entry":        `{"` + test.collection + `":[` + strings.TrimSuffix(test.forbidden, "}"),
			"broken sibling syntax": `{"` + test.collection + `":[` + test.forbidden + `,{"action":`,
		}
		for _, key := range []string{test.collection, strings.ToUpper(test.collection)} {
			for _, replacement := range []string{`5`, `1e1000`, `[]`} {
				payloads[key+" first "+replacement] = `{"` + key + `":` + replacement + `,"` + test.collection + `":[` + test.forbidden + `]}`
				payloads[key+" last "+replacement] = `{"` + test.collection + `":[` + test.forbidden + `],"` + key + `":` + replacement + `}`
			}
		}
		for _, field := range []string{"action", "kind"} {
			if field == "kind" && test.fence == trackerFence {
				continue
			}
			permitted := `"read"`
			if test.fence == artifact.WriteFence {
				permitted = `"revise"`
				if field == "kind" {
					permitted = `"goals"`
				}
			}
			for _, key := range []string{field, strings.ToUpper(field)} {
				for _, replacement := range []string{`5`, `1e1000`, permitted} {
					payloads[key+" first "+replacement] = `{"` + test.collection + `":[{"` + key + `":` + replacement + `,` + strings.TrimPrefix(test.forbidden, "{") + `]}`
					payloads[key+" last "+replacement] = `{"` + test.collection + `":[` + strings.TrimSuffix(test.forbidden, "}") + `,"` + key + `":` + replacement + `}]}`
				}
			}
		}
		for name, payload := range payloads {
			t.Run(test.name+"/"+name, func(t *testing.T) {
				block := test.fence + "\n" + payload + "\n```\n"
				provider := &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: block +
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
					t.Fatalf("Send() = %v, want the unauthorized request to refuse the whole reply", err)
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

func TestMalformedRevisionsCannotHideRecordedOwnership(t *testing.T) {
	t.Parallel()
	revision := `{"action":"revise","id":"v1-design","body":"replacement","reason":"change it"}`
	block := func(payload string) string {
		return artifact.WriteFence + "\n" + payload + "\n```\n"
	}
	blocks := map[string]string{
		"valid revision":        block(`{"documents":[` + revision + `]}`),
		"invalid sibling first": block(`{"documents":[{"action":5},` + revision + `]}`),
		"invalid sibling last":  block(`{"documents":[` + revision + `,{"action":5}]}`),
		"invalid body":          block(`{"documents":[{"action":"revise","id":"v1-design","body":5}]}`),
		"missing arguments":     block(`{"documents":[{"action":"revise","id":"v1-design"}]}`),
		"misleading kind":       block(`{"documents":[{"action":"revise","id":"v1-design","kind":"goals"}]}`),
		"unclosed entry":        block(`{"documents":[` + strings.TrimSuffix(revision, "}")),
		"broken sibling syntax": block(`{"documents":[` + revision + `,{"action":`),
		"unclosed fence":        strings.TrimSuffix(block(`{"documents":[`+revision+`]}`), "```\n"),
		"trailing opener":       strings.Replace(block(`{"documents":[`+revision+`]}`), artifact.WriteFence, artifact.WriteFence+" invalid", 1),
		"trailing closer":       strings.TrimSuffix(block(`{"documents":[`+revision+`]}`), "```\n") + "``` invalid\n",
		"repeated blocks first": block(`{"documents":[`+revision+`]}`) + documentReply("revise", "v1-goals", "", "", "replacement"),
		"repeated blocks last":  documentReply("revise", "v1-goals", "", "", "replacement") + block(`{"documents":[`+revision+`]}`),
	}
	for _, key := range []string{"id", "ID", "action", "ACTION", "documents", "DOCUMENTS"} {
		replacements := []string{`5`, `"v1-goals"`, `"missing"`}
		if strings.EqualFold(key, "action") {
			replacements = []string{`5`, `"create"`}
		} else if strings.EqualFold(key, "documents") {
			replacements = []string{`5`, `[]`, `[{"action":"revise","id":"missing"}]`}
		}
		for _, replacement := range replacements {
			if strings.EqualFold(key, "documents") {
				blocks[key+" first "+replacement] = block(`{"` + key + `":` + replacement + `,"documents":[` + revision + `]}`)
				blocks[key+" last "+replacement] = block(`{"documents":[` + revision + `],"` + key + `":` + replacement + `}`)
			} else {
				blocks[key+" first "+replacement] = block(`{"documents":[{"` + key + `":` + replacement + `,` + strings.TrimPrefix(revision, "{") + `]}`)
				blocks[key+" last "+replacement] = block(`{"documents":[` + strings.TrimSuffix(revision, "}") + `,"` + key + `":` + replacement + `}]}`)
			}
		}
	}
	for name, block := range blocks {
		t.Run(name, func(t *testing.T) {
			provider := &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: block +
				memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`) +
				trackerReply("Closing it.", `{"action":"close","id":"yoyodyne-ifd.22","reason":"finished"}`)}}}
			options, repository := documentOptions(t, provider)
			store := options.Documents.(artifact.Store)
			for _, draft := range []artifact.Draft{
				{ID: "v1-design", Kind: artifact.KindDesign, Title: "How it is built", Directory: "docs/designs", Body: "# Design", Reason: "recorded"},
				{ID: "v1-goals", Kind: artifact.KindGoals, Title: "What it serves", Directory: "docs/product", Body: "# Goals", Reason: "recorded"},
			} {
				owner, _ := artifact.Owner(draft.Kind)
				if _, err := store.Create(owner, draft, fixedClock{}.Now()); err != nil {
					t.Fatal(err)
				}
			}
			designPath := filepath.Join(repository, "docs", "designs", "v1-design.md")
			before, err := os.ReadFile(designPath)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			memories, err := runstate.NewMemoryStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			tracker := &fakeTracker{}
			options.Store, options.Memories, options.Tracker = newTestStore(t, root), memories, tracker
			session := openTestSession(t, options)
			reply, err := session.Send(context.Background(), "Carry on.")
			var unauthorized *AuthorityError
			if !errors.As(err, &unauthorized) || !strings.Contains(err.Error(), "v1-design") {
				t.Fatalf("Send() = %v, want recorded ownership to refuse the whole reply", err)
			}
			if len(reply.Actions) != 0 || len(tracker.closed) != 0 || len(reply.Memories) != 0 || len(reply.Writes) != 0 ||
				len(session.Writes()) != 0 || len(reply.BlockRefusals) != 0 || len(provider.requests) != 1 {
				t.Fatalf("unauthorized reply carried out other blocks: %+v", reply)
			}
			all, problems, err := memories.Memories(string(domain.RoleProductManager))
			if err != nil || len(problems) != 0 || len(all) != 0 {
				t.Fatalf("memory store = %v, %v, %v; want no memory written", all, problems, err)
			}
			after, err := os.ReadFile(designPath)
			if err != nil || string(after) != string(before) {
				t.Fatalf("the design changed under a refused revision: %v", err)
			}
		})
	}
}

func TestMalformedOwnedOrMissingRevisionsStillRefuseOnlyTheirBlock(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"v1-design", "missing"} {
		t.Run(id, func(t *testing.T) {
			provider := &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: artifact.WriteFence +
				"\n" + `{"documents":[{"action":"revise","id":"` + id + `"},{"action":5}]}` + "\n```\n" +
				memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`)}}}
			options, _ := documentOptions(t, provider)
			options.Role, options.Agent = domain.RoleArchitect, string(domain.RoleArchitect)
			if _, err := options.Documents.(artifact.Store).Create(domain.RoleArchitect, artifact.Draft{
				ID: "v1-design", Kind: artifact.KindDesign, Title: "How it is built", Directory: "docs/designs", Body: "# Design", Reason: "recorded",
			}, fixedClock{}.Now()); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			memories, err := runstate.NewMemoryStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			options.Store, options.Memories = newTestStore(t, root), memories
			reply, err := openTestSession(t, options).Send(context.Background(), "Carry on.")
			requireBlockRefusal(t, reply, err, "yoyodyne-artifact")
			all, problems, err := memories.Memories(string(domain.RoleArchitect))
			if err != nil || len(problems) != 0 || len(all) != 1 || len(reply.Memories) != 1 || len(reply.Writes) != 0 {
				t.Fatalf("memory did not survive the artifact validation refusal: %+v, %v, %v, %v", reply, all, problems, err)
			}
		})
	}
}

type unavailableRevisionDocuments struct{ Documents }

func (unavailableRevisionDocuments) AuthorizeRevisions(domain.AgentRole, []string) error {
	return errors.New("recorded ownership is unavailable")
}

func TestRevisionOwnershipReadFailureCarriesOutNoOtherBlocks(t *testing.T) {
	t.Parallel()
	provider := &fakeBackend{results: []backendapi.RunResult{{SessionID: "session-1", FinalText: artifact.WriteFence +
		"\n" + `{"documents":[{"action":"revise","id":"v1-design"},{"action":5}]}` + "\n```\n" +
		memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`)}}}
	options, _ := documentOptions(t, provider)
	options.Documents = unavailableRevisionDocuments{options.Documents}
	reply, err := openTestSession(t, options).Send(context.Background(), "Carry on.")
	if err == nil || !strings.Contains(err.Error(), "recorded ownership is unavailable") || len(reply.Memories) != 0 || len(reply.Writes) != 0 || len(reply.BlockRefusals) != 0 {
		t.Fatalf("ownership read failure carried out other blocks: %+v, %v", reply, err)
	}
}

func TestRepeatedFieldsWithInvalidTypesStillFailValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ fence, payload string }{
		{trackerFence, `{"actions":[{"action":"read","ACTION":5}]}`},
		{trackerFence, `{"actions":[{"action":"read"}],"ACTIONS":5}`},
		{artifact.WriteFence, `{"documents":[{"action":"create","kind":"goals","KIND":5}]}`},
		{artifact.WriteFence, `{"documents":[{"action":"create","kind":"goals"}],"DOCUMENTS":5}`},
		{trackerFence, `{"actions":[{"action":"read","nested":{"action":"reprioritize"}}]}`},
		{artifact.WriteFence, `{"documents":[{"action":"create","kind":"goals","nested":{"kind":"design"}}]}`},
	} {
		parsed, err := splitReply(domain.RoleProductManager, test.fence+"\n"+test.payload+"\n```\n"+
			memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`))
		if parsed.AuthorityProblem != nil || (err == nil && len(parsed.Refusals) == 0) ||
			len(parsed.Actions) != 0 || len(parsed.Writes) != 0 || len(parsed.Memories) != 1 {
			t.Fatalf("validation lost a block or widened authority: %+v, %v", parsed, err)
		}
	}
}

func TestAuthorizedMalformedBlocksStillFailValidation(t *testing.T) {
	t.Parallel()
	for _, block := range []string{
		trackerReply("Reading it.", `{"action":"read","id":"yoyodyne-ifd.22"}`),
		documentReply("create", "v2-goals", "goals", "docs/product", "# Goals\\n\\nShip the thing."),
	} {
		for _, malformed := range []string{
			block + block,
			strings.TrimSuffix(block, "```\n"),
			strings.Replace(block, "\n{", " invalid\n{", 1),
			strings.Replace(block, "]}", `,{"action":5}]}`, 1),
			strings.Replace(block, "]}", `,{"action":"create","kind":5}]}`, 1),
		} {
			parsed, err := splitReply(domain.RoleProductManager, malformed+
				memoryBlock(`{"memories":[{"action":"remember","memory":"lesson","text":"keep this"}]}`))
			if parsed.AuthorityProblem != nil || (err == nil && len(parsed.Refusals) == 0) ||
				len(parsed.Actions) != 0 || len(parsed.Writes) != 0 || len(parsed.Memories) != 1 {
				t.Fatalf("validation lost a block or widened authority: %+v, %v", parsed, err)
			}
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
