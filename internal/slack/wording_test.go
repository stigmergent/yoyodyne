package slack

import (
	"context"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/readmodel"
)

func TestEverySlackPostFlagsRetiredWordsEvenWithoutATracker(t *testing.T) {
	t.Parallel()
	posts := &recordedPosts{}
	sink := newTestSink(t, t.TempDir(), &fixedFeed{}, posts)
	sink.sources = &readmodel.Sources{Repository: "../.."}
	for _, channel := range []string{"channel", "direct-message"} {
		for _, thread := range []string{"", "thread"} {
			original := "post-mortem: the posture changed"
			if _, err := sink.post(context.Background(), Message{Channel: channel, ThreadTS: thread, Text: original}); err != nil {
				t.Fatal(err)
			}
			post := posts.requests[len(posts.requests)-1]
			if !strings.HasPrefix(post.Text, original) || !strings.Contains(post.Text, `[wording: "posture" was replaced; write tool access`) {
				t.Fatalf("post = %+v, want the language flag on every kind of post", post)
			}
			if post.Channel != channel || post.ThreadTS != thread {
				t.Fatalf("checking changed where the post went: %+v", post)
			}
		}
	}
	flagged := readmodel.ReadTextTerms("../..").Render("the posture changed")
	if _, err := sink.post(context.Background(), Message{Channel: "channel", Text: flagged}); err != nil {
		t.Fatal(err)
	}
	if text := posts.requests[len(posts.requests)-1].Text; text != flagged {
		t.Fatalf("an already flagged sentence acquired another flag: %q", text)
	}
}
