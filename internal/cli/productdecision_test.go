package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/chat"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/contextbundle"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

const (
	supersededItem  = "yoyodyne-ifd.428.34"
	supersedingItem = "yoyodyne-ifd.398"
)

// The relay of 2026-09-27, made by the harness: the Lead Product Manager records
// that an item with a run in flight is superseded, the decision is docketed for
// the development manager at once, her briefing puts it in front of her with the
// run's state, `yoyo status` names it while it is undecided, and the stop she
// records is carried out as any stop she decides — the request the run reads,
// written in her name — and closes the entry.
func TestAProductDecisionAboutWorkInFlightReachesTheDevelopmentManagerAndHerStopIsCarriedOut(t *testing.T) {
	// Not parallel: the state root the components read is set here.
	t.Setenv("YOYODYNE_STATE_HOME", t.TempDir())
	parts, err := buildComponents(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("buildComponents() error = %v", err)
	}
	running := recordedRun(t, parts.store, runstate.StatusRunning, supersededItem, time.Now().UTC())

	// The Lead Product Manager decides, naming the item and not the run.
	tracker := &openItemTracker{id: supersededItem, title: "the dashboard's old card"}
	product := productManagerSession(t, parts, &scriptedChatBackend{replies: []string{
		"398 does this whole, so this run should not land.\n\n```yoyodyne-tracker\n{\"actions\":[" +
			`{"action":"inflight","id":"` + supersededItem + `","decision":"superseded","superseded_by":"` + supersedingItem + `","reason":"yoyodyne-ifd.398 rebuilds this card whole, so this run's change would be thrown away"}` +
			"]}\n```\n",
	}}, tracker)
	decided := sendTriageTurn(t, product, "428.34 is superseded by 398.")
	if !decided.Applied {
		t.Fatalf("the decision was not recorded: %#v", decided)
	}
	if !strings.Contains(decided.Summary, "run "+running.RunID) || !strings.Contains(decided.Summary, "docketed it for the development manager") {
		t.Fatalf("she was not told the decision was docketed about the run in flight: %s", decided.Summary)
	}
	notes := strings.Join(tracker.notes, "\n")
	for _, want := range []string{"Decided superseded by " + supersedingItem, "while run " + running.RunID + " is in flight", "by the Lead Product Manager in conversation"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("the item's notes are missing %q:\n%s", want, notes)
		}
	}

	// The docket entry, as the development manager's docket is built.
	built, err := docketerFrom(parts).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	var entry *triage.Entry
	for index := range built.Entries {
		if built.Entries[index].Class == triage.ClassProductDecision {
			entry = &built.Entries[index]
		}
	}
	if entry == nil {
		t.Fatalf("docket = %#v, want the product decision on it", built.Entries)
	}
	if entry.RunID != running.RunID || entry.WorkItemID != supersededItem || entry.ProductDecision.SupersededBy != supersedingItem ||
		!entry.ProductDecision.RunInFlight || !strings.HasPrefix(entry.ProductDecision.DecidedBy, "the Lead Product Manager in conversation ") {
		t.Fatalf("entry = %#v / %#v, want the run, the item, what supersedes it, who decided, and the run in flight", entry, entry.ProductDecision)
	}

	// Her briefing: the docket section her sweep and her conversation are both
	// given, with the decision, the run's state, and the two answers.
	briefing, _ := contextbundle.TriageDocket(contextbundle.ProductRequest{TriageDocket: built.Entries, TriageDocketAt: time.Now()})
	for _, want := range []string{
		"[product decision about a run in flight]",
		supersededItem + " is superseded by " + supersedingItem,
		"this run's change would be thrown away",
		"Where the run stands: run " + running.RunID + " is running",
		`"stop" asks it to stop`, `"proceed" lets it finish`,
	} {
		if !strings.Contains(briefing, want) {
			t.Fatalf("her briefing is missing %q:\n%s", want, briefing)
		}
	}

	// `yoyo status`, from the shared read model, names it while it is undecided.
	sources := readmodel.Sources{Runs: parts.store, Docket: parts.docket}
	if attention := productDecisionAttentionOf(readmodel.ReadStanding(context.Background(), sources)); attention == nil || attention.ID != running.RunID ||
		attention.Mover != readmodel.MoverDevelopmentManager || !strings.Contains(attention.What(), "superseded by "+supersedingItem) {
		t.Fatalf("attention = %#v, want the undecided decision named as the development manager's", attention)
	}

	// She decides: the run stops.
	manager := developmentManagerSessionDeciding(t, parts, &scriptedChatBackend{replies: []string{
		"Stopping it.\n\n```yoyodyne-tracker\n{\"actions\":[" +
			`{"action":"triage","id":"` + supersededItem + `","run":"` + running.RunID + `","decision":"stop","superseded_by":"` + supersedingItem + `","reason":"the Lead Product Manager has superseded it; nothing it builds will land"}` +
			"]}\n```\n",
	}}, tracker)
	stopped := sendTriageTurn(t, manager, "Work the docket.")
	if !stopped.Applied {
		t.Fatalf("the stop was not carried out: %#v", stopped)
	}
	if !strings.Contains(stopped.Summary, "docket entry(s) of that run are closed") {
		t.Fatalf("the stop did not close the product decision it answered: %s", stopped.Summary)
	}
	request, requested, err := parts.store.StopRequested(running.RunID)
	if err != nil || !requested {
		t.Fatalf("StopRequested() = %v, %v; want the request the run reads", requested, err)
	}
	if request.Decision != runstate.TriageDecisionStop || !strings.HasPrefix(request.StoppedBy(), "the development manager in conversation ") ||
		!strings.Contains(request.Reason, supersedingItem) {
		t.Fatalf("request = %#v, want her stop, in her name, naming what supersedes the run", request)
	}
	counters, err := parts.store.Triage().Counters(supersededItem)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	if decision, found := counters.DecisionOf(running.RunID); !found || decision.Decision != runstate.TriageDecisionStop {
		t.Fatalf("decision = %#v (found=%t), want her stop on the item's triage record", decision, found)
	}
	if !strings.Contains(strings.Join(tracker.notes, "\n"), "Triaged: stopped in flight") {
		t.Fatalf("the item's notes do not say she stopped the run:\n%s", strings.Join(tracker.notes, "\n"))
	}

	// Decided, the entry is off her docket and off `yoyo status`.
	rebuilt, err := docketerFrom(parts).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, open := range rebuilt.Entries {
		if open.Class == triage.ClassProductDecision {
			t.Fatalf("the decided product decision is still on her docket: %#v", open)
		}
	}
	if attention := productDecisionAttentionOf(readmodel.ReadStanding(context.Background(), sources)); attention != nil {
		t.Fatalf("attention = %#v, want nothing once she has decided", attention)
	}
}

// A decision about an item with nothing in flight is refused, and nothing is
// docketed: a decision about the item alone is hers to make directly.
func TestAProductDecisionIsRefusedWhereNoRunIsInFlight(t *testing.T) {
	t.Setenv("YOYODYNE_STATE_HOME", t.TempDir())
	parts, err := buildComponents(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("buildComponents() error = %v", err)
	}
	recordedRun(t, parts.store, runstate.StatusSucceeded, supersededItem, time.Now().UTC())

	tracker := &openItemTracker{id: supersededItem, title: "the dashboard's old card"}
	product := productManagerSession(t, parts, &scriptedChatBackend{replies: []string{
		"Narrowing it.\n\n```yoyodyne-tracker\n{\"actions\":[" +
			`{"action":"inflight","id":"` + supersededItem + `","decision":"narrowed","reason":"only the card's header is still wanted"}` +
			"]}\n```\n",
	}}, tracker)
	refused := sendTriageTurn(t, product, "Narrow 428.34.")
	if refused.Applied || !strings.Contains(refused.Failure, "no run is in flight on "+supersededItem) {
		t.Fatalf("outcome = %#v, want the decision refused for want of a run in flight", refused)
	}
	if len(tracker.notes) != 0 {
		t.Fatalf("notes = %q, want nothing written", tracker.notes)
	}
	entries, err := parts.docket.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("docket = %#v, want nothing docketed", entries)
	}
}

// Only the Lead Product Manager's conversation is wired the hand that dockets a
// decision about work in flight.
func TestOnlyTheLeadProductManagerIsWiredTheHandThatDocketsAProductDecision(t *testing.T) {
	t.Setenv("YOYODYNE_STATE_HOME", t.TempDir())
	parts, err := buildComponents(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("buildComponents() error = %v", err)
	}
	for _, role := range []domain.AgentRole{
		domain.RoleDevelopmentManager, domain.RoleArchitect, domain.RoleProgramManager, domain.RoleDeveloper, domain.RoleReviewer,
	} {
		if conversationInFlight(parts, role) != nil {
			t.Fatalf("the %s was wired the hand that dockets a product decision", role)
		}
	}
	if conversationInFlight(parts, domain.RoleProductManager) == nil {
		t.Fatal("the Lead Product Manager was wired no way to docket a decision about work in flight")
	}
}

func productDecisionAttentionOf(standing readmodel.Standing) *readmodel.Attention {
	for index := range standing.NeedsHuman {
		if standing.NeedsHuman[index].Kind == readmodel.AttentionProductDecision {
			return &standing.NeedsHuman[index]
		}
	}
	return nil
}

// productManagerSession is the Lead Product Manager's conversation wired the way
// the command line wires hers for a decision about work in flight.
func productManagerSession(t *testing.T, parts components, provider chat.Backend, tracker chat.Tracker) *chat.Session {
	t.Helper()

	root := t.TempDir()
	store, err := runstate.NewConversationStore(root, "yoyodyne")
	if err != nil {
		t.Fatalf("NewConversationStore() error = %v", err)
	}
	session, err := chat.Open(chat.Options{
		Role:         domain.RoleProductManager,
		Agent:        "product-manager",
		Backend:      provider,
		Store:        store,
		Tracker:      tracker,
		InFlight:     conversationInFlight(parts, domain.RoleProductManager),
		Model:        "opus",
		Provider:     domain.BackendClaudeCode,
		AccountAlias: config.DefaultAccountAlias,
		Repository:   filepath.Join(root, "repository"),
		ProductID:    "yoyodyne",
		RepositoryID: "yoyodyne",
		Briefing:     chat.Briefing{Text: "the product is a harness", GatheredAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("chat.Open() error = %v", err)
	}
	return session
}

// developmentManagerSessionDeciding is her conversation wired as the command
// line wires it for a decision about a run in flight: the triage record, the run
// records, the hand that stops a run, and the docket her decision closes.
func developmentManagerSessionDeciding(t *testing.T, parts components, provider chat.Backend, tracker chat.Tracker) *chat.Session {
	t.Helper()

	root := t.TempDir()
	store, err := runstate.NewConversationStore(root, "yoyodyne")
	if err != nil {
		t.Fatalf("NewConversationStore() error = %v", err)
	}
	reports, err := runstate.NewReportStore(root, "yoyodyne")
	if err != nil {
		t.Fatalf("NewReportStore() error = %v", err)
	}
	session, err := chat.Open(chat.Options{
		Role:         domain.RoleDevelopmentManager,
		Agent:        string(domain.RoleDevelopmentManager),
		Backend:      provider,
		Store:        store,
		Reports:      reports,
		Tracker:      tracker,
		Triage:       conversationTriage(parts, domain.RoleDevelopmentManager),
		Stoppages:    conversationStoppages(parts, domain.RoleDevelopmentManager),
		Stops:        conversationStops(parts, domain.RoleDevelopmentManager),
		Docket:       conversationDocketEntries(parts, domain.RoleDevelopmentManager),
		Model:        "opus",
		Provider:     domain.BackendClaudeCode,
		AccountAlias: config.DefaultAccountAlias,
		Repository:   filepath.Join(root, "repository"),
		ProductID:    "yoyodyne",
		RepositoryID: "yoyodyne",
		Briefing:     chat.Briefing{Text: "the product is a harness", GatheredAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("chat.Open() error = %v", err)
	}
	return session
}
