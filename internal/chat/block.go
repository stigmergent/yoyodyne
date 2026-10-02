package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/artifact"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/evaluation"
	"github.com/mason-bryant/yoyodyne/internal/exchange"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/report"
	"github.com/mason-bryant/yoyodyne/internal/repositoryread"
	"github.com/mason-bryant/yoyodyne/internal/research"
)

// BlockRefusal is a block the role must correct, without repeating the blocks
// that were carried out. It contains the harness's reason, never the payload.
type BlockRefusal struct {
	Block   string `json:"block"`
	Problem string `json:"problem"`
}

const blockRefusalClause = "A malformed or invalid block is refused on its own: other valid blocks in your reply are carried out. The harness records the reason and tells you on your next turn; correct only the refused block. A block or action you hold no authority for refuses your whole reply before anything is carried out."

func blockRefusal(fence string, err error) BlockRefusal {
	return BlockRefusal{Block: strings.TrimPrefix(fence, "```"), Problem: err.Error()}
}

var replyFences = []string{
	report.Fence, laneReportFence, trackerFence, proposalFence, concernFence,
	research.Fence, evaluation.Fence, repositoryread.Fence, exchange.Fence,
	memoryFence, restartFence, artifact.WriteFence,
}

// replyBlocks separates framing before any payload is decoded. An unclosed
// block ends at the next recognized opener, so it cannot swallow another kind
// of block. Occurrences stay separate for authority checks; validation still
// decodes repeated blocks of one kind together and refuses them whole.
func replyBlocks(answer string) (string, map[string][]string) {
	builders := make(map[string][]*strings.Builder)
	var prose strings.Builder
	var current *strings.Builder
	for _, line := range strings.SplitAfter(answer, "\n") {
		var opening string
		for _, fence := range replyFences {
			if strings.HasPrefix(line, fence) {
				opening = fence
				break
			}
		}
		if opening != "" {
			current = &strings.Builder{}
			builders[opening] = append(builders[opening], current)
		} else if strings.HasPrefix(line, "```yoyodyne-") {
			// Blocks interpreted by the caller, such as a scheduled sweep's
			// account, stay in prose even after an unclosed conversation block.
			current = nil
		}
		if current == nil {
			prose.WriteString(line)
			continue
		}
		current.WriteString(line)
		if opening == "" && strings.HasPrefix(line, "```") {
			current = nil
		}
	}
	blocks := make(map[string][]string, len(builders))
	for fence, occurrences := range builders {
		for _, builder := range occurrences {
			blocks[fence] = append(blocks[fence], builder.String())
		}
	}
	return strings.TrimSpace(prose.String()), blocks
}

// Each kind uses its existing strict decoder, but its failure no longer stops
// the other decoders. The tracker retains its immediate correction mechanism.
func splitReply(role domain.AgentRole, answer string) (parsedReply, error) {
	prose, blocks := replyBlocks(answer)
	parsed := parsedReply{Prose: prose, Carried: make(map[string]bool)}
	var trackerProblem error
	for _, fence := range replyFences {
		occurrences, found := blocks[fence]
		if !found {
			continue
		}
		parsed.Carried[fence] = true
		for _, block := range occurrences {
			if parsed.AuthorityProblem == nil {
				parsed.AuthorityProblem = blockAuthority(role, fence, block)
			}
		}
		part, err := splitSingleReply(role, strings.Join(occurrences, ""))
		if err != nil {
			if fence == trackerFence {
				trackerProblem = err
			} else {
				parsed.Refusals = append(parsed.Refusals, blockRefusal(fence, err))
			}
			continue
		}
		switch fence {
		case report.Fence:
			parsed.Reports, parsed.ReportProblem = part.Reports, part.ReportProblem
			if part.ReportProblem != nil {
				parsed.Refusals = append(parsed.Refusals, blockRefusal(fence, part.ReportProblem))
			}
		case laneReportFence:
			parsed.LaneReport = part.LaneReport
			parsed.LaneReportCarried = part.LaneReportCarried
			parsed.LaneReportProblem = part.LaneReportProblem
		case trackerFence:
			parsed.Actions = part.Actions
		case proposalFence:
			parsed.Proposals = part.Proposals
		case concernFence:
			parsed.Concerns = part.Concerns
		case research.Fence:
			parsed.Queries = part.Queries
		case evaluation.Fence:
			parsed.Evaluation = part.Evaluation
		case repositoryread.Fence:
			parsed.Reads = part.Reads
		case exchange.Fence:
			parsed.Ask = part.Ask
		case memoryFence:
			parsed.Memories = part.Memories
		case restartFence:
			parsed.Restart = part.Restart
		case artifact.WriteFence:
			parsed.Writes = part.Writes
		}
	}
	return parsed, trackerProblem
}

// Presence is enough to check a block's capability, including malformed JSON.
// Decoded actions and document kinds still pass the finer checks in authorize.
func (s *Session) authorizeCarried(parsed parsedReply) error {
	if parsed.AuthorityProblem != nil {
		return parsed.AuthorityProblem
	}
	a := s.authority()
	permitted := map[string]bool{
		report.Fence: true, laneReportFence: a.LaneReport,
		trackerFence: len(a.TrackerActions) > 0, proposalFence: a.Proposals,
		concernFence: a.Concerns, research.Fence: a.Research,
		evaluation.Fence: a.Evaluations, repositoryread.Fence: a.RepositoryReads,
		exchange.Fence: a.Asks, memoryFence: a.Memory,
		restartFence: a.RestartRequests, artifact.WriteFence: len(artifact.Owned(a.Role)) > 0,
	}
	for _, fence := range replyFences {
		if parsed.Carried[fence] && !permitted[fence] {
			return &AuthorityError{Role: a.Role, Refused: strings.TrimPrefix(fence, "```"),
				Reason: "this role holds no authority for this block"}
		}
	}
	return nil
}

func (s *Session) recordBlockRefusal(reply *Reply, refusal BlockRefusal) error {
	// A validation refusal includes the correction the role needs to make;
	// the shorter action-failure bound can cut that guidance off.
	refusal.Problem = boundText(refusal.Problem, maxTrackerRefusalBytes)
	reply.BlockRefusals = append(reply.BlockRefusals, refusal)
	if err := s.emit(execution.EventReplyBlockRefused, map[string]any{
		"turn": s.state.Turns, "pass": s.pass, "block": refusal.Block, "problem": refusal.Problem,
	}); err != nil {
		return fmt.Errorf("record the refused %s block: %w", refusal.Block, err)
	}
	message := fmt.Sprintf("# Refused reply block\n\nYour %s block was refused: %s\nNothing in that block happened. Other valid blocks were carried out; correct only the refused block.\n\n", refusal.Block, refusal.Problem)
	s.state.PendingBlockRefusals = boundText(s.state.PendingBlockRefusals+message, maxPendingResultBytes)
	return s.record()
}

// RefusalProblems is the account a scheduled pass keeps beside its decisions.
func (r Reply) RefusalProblems() string {
	var problems []string
	for _, refusal := range r.BlockRefusals {
		problems = append(problems, refusal.Block+": "+refusal.Problem)
	}
	problems = append(problems, r.HandedBack...)
	if refusal := r.LaneReport.Refusal(); refusal != "" {
		problems = append(problems, refusal)
	}
	return strings.Join(problems, "; ")
}

func (s *Session) reportBlockRefusals(out io.Writer, reply Reply) {
	for _, refusal := range reply.BlockRefusals {
		fmt.Fprintf(out, "[refused] %s: %s\nNothing in that block happened.\n\n", refusal.Block, refusal.Problem)
	}
}

// Read authority-bearing fields separately from payload and fence validation.
// Each occurrence starts at its opening line; disregard that line's trailing
// text, or read JSON placed directly after the opener, without requiring a
// closing fence or EOF.
// Malformed framing or another entry's invalid field type must not conceal a
// known action the role may not take.
func blockAuthority(role domain.AgentRole, fence, block string) error {
	if fence != trackerFence && fence != artifact.WriteFence {
		return nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(block, fence))
	if !strings.HasPrefix(payload, "{") {
		payload = block[lineEnd(block):]
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	authority, _ := AuthorityFor(role)
	if fence == trackerFence {
		var document struct {
			Actions []json.RawMessage `json:"actions"`
		}
		if decoder.Decode(&document) != nil {
			return nil
		}
		for _, entry := range document.Actions {
			var action struct {
				Action string `json:"action"`
			}
			if json.Unmarshal(entry, &action) != nil {
				continue
			}
			if _, known := trackerCapabilities[action.Action]; known && !authority.MayAct(action.Action) {
				return &AuthorityError{Role: role, Refused: fmt.Sprintf("the %q tracker action", action.Action),
					Reason: "this role may ask for " + renderActions(authority.TrackerActions)}
			}
		}
		return nil
	}
	var document struct {
		Documents []json.RawMessage `json:"documents"`
	}
	if decoder.Decode(&document) != nil {
		return nil
	}
	for _, entry := range document.Documents {
		var write struct {
			Action artifact.WriteAction `json:"action"`
			Kind   artifact.Kind        `json:"kind"`
		}
		if json.Unmarshal(entry, &write) != nil {
			continue
		}
		if write.Action == artifact.WriteCreate && write.Kind.Valid() {
			if err := (artifact.Write{Action: write.Action, Kind: write.Kind}).Authorize(role); err != nil {
				return &AuthorityError{Role: role, Refused: "a document to be written", Reason: err.Error()}
			}
		}
	}
	return nil
}
