package slack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/report"
)

// listedTitles is a tracker listing for the sink, counting how often it was
// asked, so a catch-up that listed the tracker once per message is visible.
type listedTitles struct {
	items []beads.WorkItem
	err   error
	asked int
}

func (l *listedTitles) read(context.Context) (*readmodel.WorkItemTitles, error) {
	l.asked++
	if l.err != nil {
		return nil, l.err
	}
	return readmodel.NewWorkItemTitles(l.items), nil
}

// A role that named work by its number alone — the lane report the operator
// read on 2026-09-26 said "434.9 and 434.3" — reaches the channel with each
// item's title beside its number, whatever the message is.
func TestEveryPostNamesEachWorkItemBesideItsTitle(t *testing.T) {
	t.Parallel()

	listing := &listedTitles{items: []beads.WorkItem{
		{ID: "yoyodyne-ifd.68.12", Title: "Replay a promotion onto the moved target"},
		{ID: "yoyodyne-ifd.434.9", Title: "Price a resumed session at what it moved by"},
		{ID: "yoyodyne-ifd.434.3", Title: "Say the provider's reset in local time"},
	}}
	posts := &recordedPosts{}
	sink := newTestSink(t, t.TempDir(), &fixedFeed{deliveries: []Delivery{
		filedReport(1, report.SeverityCritical, "blocked on 434.9 and 434.3, and yoyodyne-ifd.999.1 is gone"),
		filedReport(2, report.SeverityCritical, "434.9 still blocks it"),
	}}, posts)
	sink.citing = &titleIndex{read: listing.read}

	if err := sink.pass(context.Background()); err != nil {
		t.Fatalf("pass() error = %v", err)
	}
	var said []string
	for _, request := range posts.requests {
		said = append(said, request.Text)
	}
	all := strings.Join(said, "\n---\n")
	for _, want := range []string{
		"yoyodyne-ifd.68.12 (Replay a promotion onto the moved target)",
		"434.9 (Price a resumed session at what it moved by)",
		"434.3 (Say the provider's reset in local time)",
		"yoyodyne-ifd.999.1 (unknown to the tracker)",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("posts = %q, want one to carry %q", said, want)
		}
	}
	// Each message is read on its own: the second report is a message of its
	// own in the thread, and a reader of it alone is owed the title too.
	if last := said[len(said)-1]; !strings.Contains(last, "434.9 (Price a resumed session at what it moved by)") {
		t.Errorf("last post = %q, want its item titled in it as well", last)
	}
	if listing.asked != 1 {
		t.Errorf("the tracker was listed %d times for one pass, want once", listing.asked)
	}
}

// A tracker that cannot be listed costs the titles and nothing else: the
// message is posted as it was written rather than calling every number unknown,
// and the tracker is not asked again for every message behind it.
func TestATrackerThatCannotBeListedPostsTheTextAsWritten(t *testing.T) {
	t.Parallel()

	listing := &listedTitles{err: errors.New("bd: database is locked")}
	now := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	index := &titleIndex{read: listing.read, now: func() time.Time { return now }}
	for range 3 {
		if got := index.cite(context.Background(), "blocked on yoyodyne-ifd.999.1"); got != "blocked on yoyodyne-ifd.999.1" {
			t.Fatalf("cite() = %q, want the text as written", got)
		}
	}
	if listing.asked != 1 {
		t.Errorf("a failing tracker was asked %d times within a minute, want once", listing.asked)
	}
	now = now.Add(titleIndexLife)
	index.cite(context.Background(), "yoyodyne-ifd.1")
	if listing.asked != 2 {
		t.Errorf("the tracker was asked %d times after the listing expired, want it asked again", listing.asked)
	}
}
