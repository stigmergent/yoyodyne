package runstate

import (
	"strings"
	"testing"
	"time"
)

// A job the forge ended itself — cancelled, timed out, never started — naming
// no file is one; a step that failed is not, even with its only annotation on
// .github, because that is what a genuine red test looks like.
func TestAFailingCheckIsInTheJobOnlyWhenTheForgeEndedItNamingNoFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		check FailingCheck
		want  bool
	}{
		{"cancelled, filed on .github", FailingCheck{Name: "adoption", Paths: []string{".github"}, Conclusion: "cancelled"}, true},
		{"timed out, naming nothing", FailingCheck{Name: "build", Conclusion: "timed_out"}, true},
		{"never started", FailingCheck{Name: "build", Paths: []string{".github"}, Conclusion: "startup_failure"}, true},
		{"a step failed, filed on .github", FailingCheck{Name: "build", Paths: []string{".github"}, Conclusion: "failure"}, false},
		{"cancelled, naming a file as well", FailingCheck{Name: "build", Paths: []string{".github", "internal/x/x_test.go"}, Conclusion: "cancelled"}, false},
		{"cancelled, naming a workflow file", FailingCheck{Name: "build", Paths: []string{".github/workflows/ci.yml"}, Conclusion: "cancelled"}, false},
		{"on the change", FailingCheck{Name: "lint", Paths: []string{"feature.txt"}, OnChange: []string{"feature.txt"}, Conclusion: "cancelled"}, false},
		{"waiting on approval", FailingCheck{Name: "build", Paths: []string{".github"}, Conclusion: "action_required"}, false},
		{"no conclusion recorded", FailingCheck{Name: "build", Paths: []string{".github"}}, false},
	}
	for _, tc := range cases {
		if got := tc.check.InTheJob(); got != tc.want {
			t.Errorf("%s: InTheJob() = %t, want %t", tc.name, got, tc.want)
		}
	}

	reading := PullRequestChecks{Failing: []FailingCheck{cases[0].check, cases[1].check}}
	if !reading.FailedInTheJob() {
		t.Error("a reading whose every failure the forge ended does not say so")
	}
	reading.Failing = append(reading.Failing, cases[3].check)
	if reading.FailedInTheJob() {
		t.Error("a reading with a failed step says the forge ended every job")
	}
	if (PullRequestChecks{}).FailedInTheJob() {
		t.Error("a reading with nothing failing says the forge ended a job")
	}
}

// A reading that still names only the check runs already sent back was taken
// before the re-run began; one naming a new check run is the re-run's own.
func TestAReadingOfCheckRunsAlreadySentBackIsAwaitingTheirRerun(t *testing.T) {
	t.Parallel()

	reading := PullRequestChecks{Failing: []FailingCheck{{Name: "adoption", Paths: []string{".github"}, Conclusion: "cancelled", CheckRun: 41}}}
	if reading.AwaitingRerun() {
		t.Fatal("a reading nothing was sent back for is awaiting a re-run")
	}
	reading.RecordRerun()
	if reading.Reruns != 1 || len(reading.RerunChecks) != 1 || reading.RerunChecks[0] != 41 {
		t.Fatalf("after one re-run: reruns = %d, check runs = %v", reading.Reruns, reading.RerunChecks)
	}
	if !reading.AwaitingRerun() {
		t.Error("a reading of the check run just sent back is not awaiting its re-run")
	}
	reading.Failing[0].CheckRun = 42
	if reading.AwaitingRerun() {
		t.Error("a reading of the re-run's own check run is still awaiting it")
	}
}

// The sentence every surface says names a job the forge ended as that, names a
// step that failed without naming a file as that, and blames neither on files
// the change does not touch.
func TestAJobTheForgeEndedAndAStepThatFailedAreDescribedAsWhatTheyAre(t *testing.T) {
	t.Parallel()

	reading := PullRequestChecks{
		HeadCommit: "69815201ad181b66f7181f2467bfeed2a2ce8ec0",
		ReadAt:     time.Date(2026, 9, 28, 4, 15, 56, 0, time.UTC),
		Failing: []FailingCheck{
			{Name: "adoption", Paths: []string{".github"}, Conclusion: "cancelled", CheckRun: 41},
			{Name: "build", Paths: []string{".github"}, Conclusion: "failure", CheckRun: 42},
		},
	}
	reading.RecordRerun()
	described := reading.Describe("main")
	for _, want := range []string{
		"adoption (the forge cancelled the job before any step failed, naming no file)",
		"build (a step failed without naming a file; the forge filed it on .github, and its log of the run says which step)",
		"level with main; run again 1 time(s) on this head",
	} {
		if !strings.Contains(described, want) {
			t.Errorf("Describe() = %q, want it to say %q", described, want)
		}
	}
	if strings.Contains(described, "does not touch") {
		t.Errorf("Describe() = %q names .github as a file", described)
	}
	if err := reading.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
	reading.Reruns = -1
	if reading.Validate() == nil {
		t.Error("Validate() accepted a negative re-run count")
	}
	reading.Reruns = 1
	reading.RerunChecks = make([]int64, maxRerunChecks+1)
	if reading.Validate() == nil {
		t.Error("Validate() accepted more re-run check runs than the bound")
	}
}
