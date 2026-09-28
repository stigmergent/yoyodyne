package cli

// yoyo agent cut-replies: every reply the product's conversation logs hold only
// the beginning of.
//
// A conversation's event log is where a role's ruling lives until it can write
// the document it owns, and until yoyodyne-ifd.430.20 a reply past 16 KiB was
// cut there with nothing but a marker at its end. Two of the architect's
// rulings were lost that way and found by a person transcribing them. This
// reads every log the product holds, the ones no record points at any more
// included, and names each reply the log holds cut, so the rest can be found
// and restated rather than rediscovered one at a time.

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

type cutRepliesReport struct {
	Product string `json:"product"`
	// Conversations is how many event logs were read, and Replies how many
	// replies they hold between them.
	Conversations int `json:"conversations"`
	Replies       int `json:"replies"`
	// Cut is how many of those replies the logs hold cut, and Affected the
	// conversations they are in.
	Cut      int                      `json:"cut"`
	Affected []cutRepliesConversation `json:"affected"`
	// Unreadable is each log, or line of one, that could not be read, because a
	// log nobody could read is not one that holds no cut reply.
	Unreadable []string `json:"unreadable,omitempty"`
}

type cutRepliesConversation struct {
	ConversationID string `json:"conversation_id"`
	// Agent is whose conversation this is, where a record still points at it.
	Agent string         `json:"agent,omitempty"`
	Cuts  []cutReplyLine `json:"cuts"`
}

type cutReplyLine struct {
	execution.ReplyCut
	RecordedAt time.Time `json:"recorded_at"`
}

func auditCutReplies(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("agent cut-replies", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "configuration file path (default: the nearest project configuration)")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	positional, err := parseArguments(flags, args)
	if err != nil {
		return 2
	}
	if len(positional) != 0 {
		fmt.Fprintln(stderr, "agent cut-replies reads every conversation and accepts no positional arguments")
		return 2
	}
	parts, err := buildComponents(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := runstate.NewConversationStore(parts.stateRoot, parts.config.Product.ID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := readCutReplies(store)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report.Product = string(parts.config.Product.ID)
	if *jsonOutput {
		return writeJSON(stdout, stderr, report)
	}
	fmt.Fprint(stdout, renderCutReplies(report))
	return 0
}

// readCutReplies reads every conversation log the store holds for replies it
// holds cut.
func readCutReplies(store *runstate.ConversationStore) (cutRepliesReport, error) {
	report := cutRepliesReport{Affected: []cutRepliesConversation{}}
	ids, err := store.LoggedConversations()
	if err != nil {
		return report, err
	}
	agents := map[string]string{}
	records, unreadable, err := store.RecordedReadable()
	if err != nil {
		return report, err
	}
	for _, record := range records {
		agents[record.ConversationID] = record.Identity().Agent
	}
	for _, problem := range unreadable {
		report.Unreadable = append(report.Unreadable, fmt.Sprintf("conversation record %s: %v", problem.Record, problem.Err))
	}
	for _, id := range ids {
		events, skipped, err := store.ScanEvents(id)
		if err != nil {
			report.Unreadable = append(report.Unreadable, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		report.Conversations++
		for _, line := range skipped {
			report.Unreadable = append(report.Unreadable, fmt.Sprintf("%s: line %d: %s", id, line.Line, line.Problem))
		}
		affected := cutRepliesConversation{ConversationID: id, Agent: agents[id]}
		for _, event := range events {
			if event.Type != execution.EventAgentMessage {
				continue
			}
			report.Replies++
			if cut, ok := execution.ReplyCutIn(event); ok {
				affected.Cuts = append(affected.Cuts, cutReplyLine{ReplyCut: cut, RecordedAt: event.Timestamp})
			}
		}
		if len(affected.Cuts) > 0 {
			report.Cut += len(affected.Cuts)
			report.Affected = append(report.Affected, affected)
		}
	}
	return report, nil
}

func renderCutReplies(report cutRepliesReport) string {
	var rendered []byte
	rendered = fmt.Appendf(rendered, "%d of %d recorded replies across %d conversation log(s) for %s are held cut, in %d conversation(s).\n",
		report.Cut, report.Replies, report.Conversations, report.Product, len(report.Affected))
	for _, conversation := range report.Affected {
		whose := "no record points at it any more"
		if conversation.Agent != "" {
			whose = "the " + conversation.Agent + " agent's"
		}
		rendered = fmt.Appendf(rendered, "\n%s (%s): %d cut\n", conversation.ConversationID, whose, len(conversation.Cuts))
		for _, cut := range conversation.Cuts {
			rendered = fmt.Appendf(rendered, "  %s  %s\n", cut.RecordedAt.Local().Format("2006-01-02 15:04 MST"), cut.Describe())
		}
	}
	if len(report.Unreadable) > 0 {
		rendered = fmt.Appendf(rendered, "\n%d thing(s) could not be read, so the count above may be short:\n", len(report.Unreadable))
		for _, problem := range report.Unreadable {
			rendered = fmt.Appendf(rendered, "  %s\n", problem)
		}
	}
	return string(rendered)
}
