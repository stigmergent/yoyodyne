package codex

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// recordedStream is what a stream under testdata/streams is expected to leave
// the parser holding. Each recorded file has one, and a recorded file with
// none fails the test, so a stream nobody has said anything about cannot sit
// in the directory looking like coverage.
type recordedStream struct {
	sessionID string
	// terminal is whether the stream carried a terminal this parser reads.
	terminal bool
	// unrecognized is the first event this parser did not know, or empty.
	unrecognized string
}

var recordedStreams = map[string]recordedStream{
	// Never reached the provider: a sandbox proxy refused every connection, so
	// the CLI reconnected until the recording was stopped. Every `error` in it
	// is a reconnect notice, and none of them ends the turn.
	"codex-cli-0.159.2/no-provider-reached.jsonl": {
		sessionID: "01a0f859-a609-71b2-8b5a-a4a52704a5df",
	},
}

func TestEveryRecordedStreamIsReadAsTheCLIWroteIt(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("testdata", "streams", "*", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no recorded Codex streams under testdata/streams")
	}
	for _, path := range paths {
		name := filepath.ToSlash(strings.TrimPrefix(path, filepath.Join("testdata", "streams")+string(filepath.Separator)))
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want, ok := recordedStreams[name]
			if !ok {
				t.Fatalf("recorded stream %s has no expectation in recordedStreams", name)
			}
			parser := newStreamParser(testRunID, domain.RoleDeveloper, 0, fixedClock{}, execution.NewRedactor(), nil, nil, Dialect{})
			for _, line := range readRecordedLines(t, path) {
				if err := parser.ParseLine(line); err != nil {
					t.Fatalf("ParseLine(%q) error = %v", line, err)
				}
			}
			result := parser.Result()
			if result.SessionID != want.sessionID {
				t.Errorf("session = %q, want %q", result.SessionID, want.sessionID)
			}
			if parser.SawTerminal() != want.terminal {
				t.Errorf("terminal = %v, want %v (stop reason %q)", parser.SawTerminal(), want.terminal, result.StopReason)
			}
			if got := parser.FirstUnrecognized(); got != want.unrecognized {
				t.Errorf("first unrecognized event = %q, want %q", got, want.unrecognized)
			}
			if !want.terminal {
				// A stream that has not ended has been answered nothing: a notice
				// read as a terminal would leave a failure, a refusal, or a wait
				// here that the provider never gave.
				if result.IsError || result.ServerOverload != nil || result.TransientFailure != nil || result.ProviderOutage != nil || result.UsageLimit != nil || result.ModelUnavailable != nil {
					t.Errorf("a stream with no terminal was given an outcome: %#v", result)
				}
			}
		})
	}
}

// A newer-vocabulary `error` is the provider retrying, whatever status its
// prose quotes; the recorded one quoted a 403, which read as a terminal is a
// refusal that stands.
func TestANoticeIsTheProviderRetrying(t *testing.T) {
	t.Parallel()

	observation, said := (Dialect{}).Observe(backendapi.ProviderEvent{
		Type: eventError,
		Text: "Reconnecting... 2/5 (stream disconnected before completion: URL error: Proxy connection failed: HTTP CONNECT failed with status 403)",
	})
	if !said || observation.Answer != backendapi.AnswerRetrying {
		t.Fatalf("Observe() = %#v, %v; want retrying", observation, said)
	}
}

// The older vocabulary's `error` is still the invocation's failed terminal; a
// bare one is what changed.
func TestAnEnvelopedErrorIsStillATerminal(t *testing.T) {
	t.Parallel()

	parser := newStreamParser(testRunID, domain.RoleDeveloper, 0, fixedClock{}, execution.NewRedactor(), nil, nil, Dialect{})
	if err := parser.ParseLine(`{"id":"1","msg":{"type":"error","message":"stream failed"}}`); err != nil {
		t.Fatal(err)
	}
	if !parser.SawTerminal() || !parser.Result().IsError {
		t.Fatalf("enveloped error was not read as a failed terminal: %#v", parser.Result())
	}
}

// An item this parser has not seen recorded is named by its item type, so the
// error a stream with no terminal fails with says which item it was. The line
// is written by hand in the shape a reviewer reported; it is not recorded.
func TestAnUnknownItemIsNamedByItsType(t *testing.T) {
	t.Parallel()

	parser := newStreamParser(testRunID, domain.RoleDeveloper, 0, fixedClock{}, execution.NewRedactor(), nil, nil, Dialect{})
	if err := parser.ParseLine(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"done"}}`); err != nil {
		t.Fatal(err)
	}
	if got, want := parser.FirstUnrecognized(), "item.completed (agent_message item)"; got != want {
		t.Fatalf("first unrecognized = %q, want %q", got, want)
	}
}

func readRecordedLines(t *testing.T, path string) []string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var read []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		read = append(read, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return read
}
