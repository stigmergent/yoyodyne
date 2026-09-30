package runstate

import (
	"os"
	"strings"
	"testing"
	"time"
)

// A listing that fails starts the record's failing moment, later failures count
// against it without moving it, and a listing that answers ends it and says
// when. A listing that answers while nothing is failing writes nothing at all.
func TestTrackerListingsSayWhenListingsStoppedAnsweringAndWhenTheyCameBack(t *testing.T) {
	t.Parallel()
	store, err := NewTrackerListingStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 30, 18, 13, 0, 0, time.UTC)

	if err := store.Answered(start.Add(-time.Minute)); err != nil {
		t.Fatalf("Answered() on no record: %v", err)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("an answer with nothing failing wrote the record: %v", err)
	}
	if read, err := store.Read(); err != nil || read.Failing() || read.Says() != "" {
		t.Fatalf("Read() with no record = %+v, %v; want answering", read, err)
	}

	if err := store.Failed(start, "bd list failed with status timed_out and exit code -1:"); err != nil {
		t.Fatal(err)
	}
	if err := store.Failed(start.Add(5*time.Minute), "bd list failed with status timed_out and exit code -1: second"); err != nil {
		t.Fatal(err)
	}
	read, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !read.FailingSince.Equal(start) || read.Failures != 2 || !read.LatestAt.Equal(start.Add(5*time.Minute)) || !strings.HasSuffix(read.Latest, "second") {
		t.Fatalf("Read() = %+v, want failing since the first failure, two failures, the second the latest", read)
	}
	if says := read.Says(); !strings.Contains(says, "since 2026-09-30T18:13:00Z") || !strings.Contains(says, "2 listing(s)") {
		t.Fatalf("Says() = %q", says)
	}

	if err := store.Answered(start.Add(10 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	read, err = store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if read.Failing() || read.Failures != 0 || !read.AnsweredAt.Equal(start.Add(10*time.Minute)) {
		t.Fatalf("Read() after an answer = %+v, want answering, and when it came back", read)
	}

	// A new failure after the answer is a new failing moment rather than the old
	// one continued.
	if err := store.Failed(start.Add(time.Hour), "again"); err != nil {
		t.Fatal(err)
	}
	if read, _ = store.Read(); !read.FailingSince.Equal(start.Add(time.Hour)) || read.Failures != 1 {
		t.Fatalf("Read() after a later failure = %+v, want a new failing moment", read)
	}
}
