package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// recordingBackend records each reply the way the backend parsers do, under
// the reply bound, so what the conversation's log holds is what a real
// provider's reply would leave there.
type recordingBackend struct {
	replies  []string
	requests []backendapi.RunRequest
}

func (f *recordingBackend) Run(_ context.Context, request backendapi.RunRequest) (backendapi.RunResult, error) {
	index := len(f.requests)
	f.requests = append(f.requests, request)
	said := "Noted."
	if index < len(f.replies) {
		said = f.replies[index]
	}
	sequence := request.LastSequence + 1
	event, err := execution.NewEvent(request.RunID, sequence, fixedClock{}.Now(),
		execution.EventAgentMessage, "provider.test", execution.ReplyPayload(said))
	if err != nil {
		return backendapi.RunResult{}, err
	}
	if err := request.EventSink(event); err != nil {
		return backendapi.RunResult{}, err
	}
	return backendapi.RunResult{SessionID: "session-1", FinalText: said, LastEvent: sequence}, nil
}

// A reply past the bound is recorded up to it with the cut declared at the point
// it falls and the reply's whole size, the operator is still handed all of it
// and told the record is short, and the role's next turn opens by saying so —
// once, and not again after a turn that was recorded whole.
func TestAReplyPastTheBoundIsDeclaredCutAndTheRoleIsToldNextTurn(t *testing.T) {
	t.Parallel()

	ruling := strings.Repeat("The ruling stands on these grounds. ", execution.MaxReplyTextBytes/36+200) +
		"Landing: the release's notes are written by the cut."
	provider := &recordingBackend{replies: []string{ruling, "Restated: the release's notes are written by the cut.", "Nothing further."}}
	options := testOptions(t, provider)
	session := openTestSession(t, options)

	reply, err := session.Send(context.Background(), "rule on the release notes")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !strings.Contains(reply.Text, "Landing: the release's notes are written by the cut.") {
		t.Fatal("the operator was not handed the whole reply")
	}
	if len(reply.RecordCuts) != 1 {
		t.Fatalf("record cuts = %#v, want the one reply", reply.RecordCuts)
	}
	cut := reply.RecordCuts[0]
	if cut.WholeBytes != len(ruling) || cut.RecordedBytes != execution.MaxReplyTextBytes {
		t.Fatalf("cut = %#v, want %d of %d bytes", cut, execution.MaxReplyTextBytes, len(ruling))
	}
	if rendered := RenderRecordCuts(reply.RecordCuts); !strings.Contains(rendered, "[record]") ||
		!strings.Contains(rendered, fmt.Sprintf("a reply of %d bytes", len(ruling))) {
		t.Fatalf("transcript line = %q, want the cut and the whole size named", rendered)
	}

	// The declaration is in the record itself, at the point the cut falls.
	events, err := options.Store.LoadEvents(session.state.ConversationID)
	if err != nil {
		t.Fatalf("LoadEvents() error = %v", err)
	}
	var recorded struct {
		Text          string `json:"text"`
		Cut           bool   `json:"cut"`
		RecordedBytes int    `json:"recorded_bytes"`
		WholeBytes    int    `json:"whole_bytes"`
	}
	for _, event := range events {
		if event.Type == execution.EventAgentMessage {
			if err := json.Unmarshal(event.Payload, &recorded); err != nil {
				t.Fatalf("decode reply: %v", err)
			}
			break
		}
	}
	marker := fmt.Sprintf("…[cut here: the record holds the first %d of this reply's %d bytes]", execution.MaxReplyTextBytes, len(ruling))
	if !recorded.Cut || recorded.WholeBytes != len(ruling) || recorded.RecordedBytes != execution.MaxReplyTextBytes ||
		recorded.Text != ruling[:execution.MaxReplyTextBytes]+marker {
		t.Fatalf("recorded reply = cut %t, %d of %d bytes, ending %q; want the cut declared",
			recorded.Cut, recorded.RecordedBytes, recorded.WholeBytes, recorded.Text[len(recorded.Text)-120:])
	}

	// The cut outlives this process: the next one to open the conversation
	// still owes the role the notice.
	stored, err := options.Store.Load(options.identity())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored.ReplyCuts) != 1 || stored.ReplyCuts[0] != cut {
		t.Fatalf("recorded cuts = %#v, want %#v", stored.ReplyCuts, cut)
	}

	resumed := openTestSession(t, options)
	if _, err := resumed.Send(context.Background(), "go on"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	told := provider.requests[1].Prompt
	if !strings.HasPrefix(told, "# Your last reply was recorded cut") {
		t.Fatalf("next turn opened %q, want the cut first", told[:min(len(told), 200)])
	}
	for _, want := range []string{
		fmt.Sprintf("event %d holds its first %d bytes", cut.Sequence, execution.MaxReplyTextBytes),
		fmt.Sprintf("a reply of %d bytes", len(ruling)),
		fmt.Sprintf("ending %q", cut.EndsWith),
	} {
		if !strings.Contains(told, want) {
			t.Fatalf("next turn does not say %q", want)
		}
	}

	if _, err := resumed.Send(context.Background(), "anything else?"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if strings.Contains(provider.requests[2].Prompt, "recorded cut") {
		t.Fatal("the role was told about the same cut twice")
	}
	if stored, _ := options.Store.Load(options.identity()); len(stored.ReplyCuts) != 0 {
		t.Fatalf("recorded cuts = %#v after the role was told, want none", stored.ReplyCuts)
	}
}

// A reply within the bound is recorded whole, and nobody is told anything.
func TestAReplyWithinTheBoundIsRecordedWhole(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 64<<10)
	provider := &recordingBackend{replies: []string{long, "short"}}
	session := openTestSession(t, testOptions(t, provider))
	reply, err := session.Send(context.Background(), "say a lot")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(reply.RecordCuts) != 0 {
		t.Fatalf("record cuts = %#v, want none for a reply the record holds", reply.RecordCuts)
	}
	if _, err := session.Send(context.Background(), "and?"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if strings.Contains(provider.requests[1].Prompt, "recorded cut") {
		t.Fatal("a reply recorded whole was reported cut")
	}
}
