package slack

// Every message the sink posts names each work item beside its title.
//
// The records the sink reads carry what roles and the harness wrote, and a role
// that names work by its number alone — "434.9 and 434.3" — would otherwise put
// that number in the channel exactly as it wrote it. So the text of every post
// is read through the read model's own resolution on its way out, which is the
// one place every message passes: a thread's opening message, a milestone, a
// digest, the hourly line and the needs-a-human line under it, a direct message,
// and an answer in a thread.
//
// The tracker is listed at most once a minute rather than once a message. A
// catch-up posts hundreds of messages in a row and titles do not change under
// it; an item admitted in the last minute is named by its number until the next
// listing, which is the same answer a tracker that could not be read gets.

import (
	"context"
	"sync"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/readmodel"
)

// titleIndexLife is how long one listing of the tracker's titles is used for.
const titleIndexLife = time.Minute

// titleIndex is the sink's listing of what the tracker calls each item. It is
// shared by every goroutine that posts, so it holds its own lock.
type titleIndex struct {
	mu     sync.Mutex
	read   func(ctx context.Context) (*readmodel.WorkItemTitles, error)
	now    func() time.Time
	titles *readmodel.WorkItemTitles
	at     time.Time
	tried  bool
}

// cite is text with every work item in it beside its title, as far as the
// latest listing knows. A listing that failed leaves text as it was.
func (t *titleIndex) cite(ctx context.Context, text string) string {
	if t == nil || t.read == nil || text == "" {
		return text
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if t.now != nil {
		now = t.now()
	}
	if !t.tried || now.Sub(t.at) >= titleIndexLife {
		// A failed listing is kept as nothing for the same minute a good one is
		// kept for, so a tracker that will not answer is not asked once per
		// message of a catch-up.
		titles, err := t.read(ctx)
		if err != nil {
			titles = nil
		}
		t.titles, t.at, t.tried = titles, now, true
	}
	return t.titles.Cite(text)
}

// sourcesTitles lists the titles from the read model's sources.
func sourcesTitles(sources *readmodel.Sources) func(ctx context.Context) (*readmodel.WorkItemTitles, error) {
	if sources == nil || sources.Tracker == nil {
		return nil
	}
	return func(ctx context.Context) (*readmodel.WorkItemTitles, error) {
		return readmodel.ReadWorkItemTitles(ctx, *sources)
	}
}
