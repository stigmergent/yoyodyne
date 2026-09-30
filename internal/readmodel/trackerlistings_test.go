package readmodel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

type fakeTrackerListings struct {
	record runstate.TrackerListings
	err    error
}

func (f fakeTrackerListings) Read() (runstate.TrackerListings, error) { return f.record, f.err }

// A tracker that is not answering listings is on the line `yoyo status`
// prints, as the harness's move, saying since when — and off it again once a
// listing answers.
func TestATrackerNotAnsweringListingsIsSaidWithSinceWhen(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 9, 30, 18, 13, 0, 0, time.UTC)
	sources := quietSources()
	sources.TrackerListings = fakeTrackerListings{record: runstate.TrackerListings{
		FailingSince: since, Failures: 2, LatestAt: since.Add(10 * time.Minute),
		Latest: "bd list did not answer within its 30s bound on any of 3 attempts over 1m40s: bd list failed with status timed_out and exit code -1:",
	}}

	standing := ReadStanding(context.Background(), sources)
	var found *Attention
	for index, entry := range standing.NeedsHuman {
		if entry.Kind == AttentionTrackerUnanswered {
			found = &standing.NeedsHuman[index]
		}
	}
	if found == nil {
		t.Fatalf("NeedsHuman = %+v, want the tracker's listings on it", standing.NeedsHuman)
	}
	if found.Mover != MoverHarness || !found.Since().Equal(since) {
		t.Fatalf("entry = %+v, want the harness's move since %s", found, since)
	}
	rendered := standing.Render()
	if !strings.Contains(rendered, "Waiting on the harness") || !strings.Contains(rendered, "the tracker has not answered a listing since 2026-09-30T18:13:00Z") {
		t.Fatalf("Render() = %q, want the tracker named under the harness with since when", rendered)
	}

	sources.TrackerListings = fakeTrackerListings{record: runstate.TrackerListings{AnsweredAt: since.Add(time.Hour)}}
	for _, entry := range ReadStanding(context.Background(), sources).NeedsHuman {
		if entry.Kind == AttentionTrackerUnanswered {
			t.Fatalf("NeedsHuman carries %+v after a listing answered", entry)
		}
	}
}

// A record that cannot be read is said as that, never as a tracker answering.
func TestAnUnreadableListingRecordIsSaidRatherThanReadAsAnswering(t *testing.T) {
	t.Parallel()
	sources := quietSources()
	sources.TrackerListings = fakeTrackerListings{err: errors.New("decode tracker listing record: bad")}
	standing := ReadStanding(context.Background(), sources)
	if !strings.Contains(standing.NeedsHumanProblem, "whether the tracker is answering listings could not be read") {
		t.Fatalf("NeedsHumanProblem = %q", standing.NeedsHumanProblem)
	}
}
