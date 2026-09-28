package report

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
)

const writtenDigest = `{"severity":"warning","message":"The line held.","digest":{"admissions":["yoyodyne-ifd.500"],"requests":[{"work":"a cache check","goal":"Run development nearly autonomously.","priority":2,"why":"three stops"}],"objections":[{"item":"yoyodyne-ifd.501","concern":"it repeats yoyodyne-ifd.480"}],"open_requests":["exchange-7"]}}`

func digestAttribution(role domain.AgentRole) Attribution {
	return Attribution{Role: role, Agent: "factory", RunID: "chat-1", ProductID: "yoyodyne", RepositoryID: "yoyodyne"}
}

// A digest is decoded from the report block, stamped with the lane and the
// pass, and filed as one report whose text is the fixed shape rendered.
func TestADigestIsFiledAsOneReportInItsFixedShape(t *testing.T) {
	t.Parallel()

	entries, err := Decode(`{"reports":[` + writtenDigest + `]}`)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	entries[0].Digest.Lane, entries[0].Digest.Pass = "factory", "factory#3"
	collected, err := Collect(entries, digestAttribution(domain.RoleProgramManager), time.Now())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(collected) != 1 || collected[0].Digest == nil {
		t.Fatalf("collected = %#v", collected)
	}
	want := strings.Join([]string{
		`Digest of the lane "factory", from pass factory#3.`,
		"The line held.",
		"Admitted in the lane this pass: yoyodyne-ifd.500.",
		"Requests outside the lane:",
		"- a cache check — would serve: Run development nearly autonomously.; recommended priority 2; why: three stops",
		"Objections to new admissions:",
		"- yoyodyne-ifd.501: it repeats yoyodyne-ifd.480",
		"Open requests from earlier passes: exchange-7.",
	}, "\n")
	if collected[0].Message != want {
		t.Fatalf("message =\n%s\nwant\n%s", collected[0].Message, want)
	}
}

// Two digests in one reply are refused whole.
func TestTwoDigestsInOneBlockAreRefused(t *testing.T) {
	t.Parallel()

	if _, err := Decode(`{"reports":[` + writtenDigest + `,` + writtenDigest + `]}`); err == nil || !strings.Contains(err.Error(), "2 digests in one reply") {
		t.Fatalf("Decode() error = %v, want two digests refused", err)
	}
}

// A digest is the program manager's alone, and one filed unstamped is refused.
func TestADigestFromAnyOtherRoleOrUnstampedIsRefused(t *testing.T) {
	t.Parallel()

	entries, err := Decode(`{"reports":[` + writtenDigest + `]}`)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if _, err := Collect(entries, digestAttribution(domain.RoleProgramManager), time.Now()); err == nil || !strings.Contains(err.Error(), "digest lane") {
		t.Fatalf("an unstamped digest was collected: %v", err)
	}
	entries[0].Digest.Lane, entries[0].Digest.Pass = "factory", "factory#3"
	if _, err := Collect(entries, digestAttribution(domain.RoleDeveloper), time.Now()); err == nil || !strings.Contains(err.Error(), "filed by the program manager alone") {
		t.Fatalf("a developer's digest was collected: %v", err)
	}
}
