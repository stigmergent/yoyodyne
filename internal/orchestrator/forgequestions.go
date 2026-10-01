package orchestrator

// Asking the forge about many publications at once.
//
// A reconcile sweep used to ask the forge about each publication it read with
// a listing of its own, and about each open one's place in the merge queue with
// a second call. On 2026-09-29 a pass over 798 recorded publications took more
// than an hour, and while it ran no other maintenance pass started, so the two
// runs a redeploy drain had preserved held both developer slots with 49 items
// ready. Most of what that hour asked about was settled long before: merged,
// closed, superseded, or handed back for a fresh run.
//
// So two things hold here. The sweeps ask only about the publications their
// own predicates call unsettled, and those predicates leave out every record
// whose answer cannot change what the harness does. And the questions that
// remain are asked in batches — one forge query per batch of up to
// publish.MaxStatesPerQuery branches — where the forge client can answer that
// way. What the sweep asked and how long the forge took is tallied, so a pass
// says it.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/publish"
)

// PublicationStates is the batched form of ReconcilePullRequests.State: one
// question about many branches. A branch with no request is absent from the
// answer. It is satisfied by publish.GitHub; a forge client without it is asked
// one branch at a time, as it always was.
type PublicationStates interface {
	States(ctx context.Context, heads []string) (map[string]publish.PullRequest, error)
}

// ForgeQuestions is what one sweep asked the forge about its publications and
// how long the forge took to answer. It counts the questions about pull
// requests — the refresh, the finishing, the recovery, and the settlement of
// queued merges — and not the reading of a queued merge's checks, which is its
// own question about a handful of runs.
type ForgeQuestions struct {
	// Publications is how many publications the forge was asked about.
	Publications int `json:"publications"`
	// Queries is how many requests that took: one per batch where the forge
	// answers in batches, one per publication where it does not.
	Queries int `json:"queries"`
	// Took is how long the forge spent answering them, all told.
	Took time.Duration `json:"took_ns"`
	// Failed is how many of those requests the forge did not answer.
	Failed int `json:"failed,omitempty"`

	mutex sync.Mutex
}

// Describe is the sweep's one line about the forge.
func (q *ForgeQuestions) Describe() string {
	if q == nil {
		return ""
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.Publications == 0 {
		return "asked the forge about no publication: none is unsettled"
	}
	said := fmt.Sprintf("asked the forge about %d unsettled publication(s) in %d request(s), which took %s",
		q.Publications, q.Queries, q.Took.Round(time.Millisecond))
	if q.Failed > 0 {
		said += fmt.Sprintf("; %d request(s) went unanswered and are asked again next sweep", q.Failed)
	}
	return said
}

func (q *ForgeQuestions) add(publications int, took time.Duration, failed bool) {
	if q == nil {
		return
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.Publications += publications
	q.Queries++
	q.Took += took
	if failed {
		q.Failed++
	}
}

// forgeAnswers is what the forge said about a set of branches, asked ahead of
// the sweep that reads them. A branch the batch did not cover is asked on its
// own when it is read.
type forgeAnswers struct {
	answered map[string]publish.PullRequest
	failed   map[string]error
	// asked holds every branch a batch covered, answered or not, so a branch the
	// forge holds no request for is told apart from one nobody asked about.
	asked map[string]bool
}

// askForge asks the forge about every branch named, in batches where the forge
// client can answer that way. A client that cannot is asked nothing here, and
// each branch is asked on its own as the sweep reads it.
func (r Reconciler) askForge(ctx context.Context, heads []string) forgeAnswers {
	answers := forgeAnswers{
		answered: map[string]publish.PullRequest{},
		failed:   map[string]error{},
		asked:    map[string]bool{},
	}
	batched, ok := r.Publisher.(PublicationStates)
	if !ok {
		return answers
	}
	unique := make([]string, 0, len(heads))
	for _, head := range heads {
		if head == "" || answers.asked[head] {
			continue
		}
		answers.asked[head] = true
		unique = append(unique, head)
	}
	for start := 0; start < len(unique); start += publish.MaxStatesPerQuery {
		end := min(start+publish.MaxStatesPerQuery, len(unique))
		batch := unique[start:end]
		if err := ctx.Err(); err != nil {
			for _, head := range batch {
				answers.failed[head] = err
			}
			continue
		}
		began := r.clock().Now()
		states, err := batched.States(ctx, batch)
		r.Forge.add(len(batch), r.clock().Now().Sub(began), err != nil)
		for _, head := range batch {
			switch {
			case err != nil:
				answers.failed[head] = err
			default:
				if state, found := states[head]; found {
					answers.answered[head] = state
				}
			}
		}
	}
	return answers
}

// state is the forge's answer about one branch: the batch's where a batch
// asked, and otherwise the forge asked now. A batch that found no request for
// the branch answers as State does about one.
func (r Reconciler) state(ctx context.Context, answers forgeAnswers, head string) (publish.PullRequest, error) {
	if answers.asked[head] {
		if err, failed := answers.failed[head]; failed {
			return publish.PullRequest{}, err
		}
		if state, found := answers.answered[head]; found {
			return state, nil
		}
		return publish.PullRequest{}, fmt.Errorf("no pull request exists for branch %s", head)
	}
	return r.askState(ctx, head)
}

// askState asks the forge about one branch on its own, and tallies it.
func (r Reconciler) askState(ctx context.Context, head string) (publish.PullRequest, error) {
	began := r.clock().Now()
	state, err := r.Publisher.State(ctx, head)
	r.Forge.add(1, r.clock().Now().Sub(began), err != nil)
	return state, err
}
