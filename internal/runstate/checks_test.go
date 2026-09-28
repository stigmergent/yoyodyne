package runstate

import (
	"strings"
	"testing"
	"time"
)

// A failure filed only on .github, or naming nothing, failed in the job; one
// naming any file in the tree, or waiting on a person's approval, did not.
func TestAFailingCheckFailedInTheJobOnlyWhenItNamesNoFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		check FailingCheck
		want  bool
	}{
		{"filed on .github", FailingCheck{Name: "adoption", Paths: []string{".github"}, Conclusion: "cancelled"}, true},
		{"naming nothing", FailingCheck{Name: "build", Conclusion: "timed_out"}, true},
		{"naming a file as well", FailingCheck{Name: "build", Paths: []string{".github", "internal/x/x_test.go"}}, false},
		{"naming a workflow file", FailingCheck{Name: "build", Paths: []string{".github/workflows/ci.yml"}}, false},
		{"on the change", FailingCheck{Name: "lint", Paths: []string{"feature.txt"}, OnChange: []string{"feature.txt"}}, false},
		{"waiting on approval", FailingCheck{Name: "build", Paths: []string{".github"}, Conclusion: "action_required"}, false},
	}
	for _, tc := range cases {
		if got := tc.check.InTheJob(); got != tc.want {
			t.Errorf("%s: InTheJob() = %t, want %t", tc.name, got, tc.want)
		}
	}

	reading := PullRequestChecks{Failing: []FailingCheck{cases[0].check, cases[1].check}}
	if !reading.FailedInTheJob() {
		t.Error("a reading whose every failure is in the job does not say so")
	}
	reading.Failing = append(reading.Failing, cases[2].check)
	if reading.FailedInTheJob() {
		t.Error("a reading with a failure naming a file says it failed only in the job")
	}
	if (PullRequestChecks{}).FailedInTheJob() {
		t.Error("a reading with nothing failing says it failed in the job")
	}
}

// The sentence every surface says names a failure in the job as that, rather
// than as a file the change does not touch, and counts the re-runs.
func TestAFailureInTheJobIsDescribedAsOne(t *testing.T) {
	t.Parallel()

	reading := PullRequestChecks{
		HeadCommit: "69815201ad181b66f7181f2467bfeed2a2ce8ec0",
		ReadAt:     time.Date(2026, 9, 28, 4, 15, 56, 0, time.UTC),
		Failing:    []FailingCheck{{Name: "adoption", Paths: []string{".github"}, Conclusion: "cancelled"}},
		Reruns:     1,
	}
	described := reading.Describe("main")
	for _, want := range []string{"adoption (the job was cancelled without naming any file; the forge filed it on .github)", "level with main; run again 1 time(s) on this head"} {
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
}
