package review

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
)

func TestReviewChecksCanonicalAbsenceClaimsAgainstRepositoryEvidence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		path      string
		listing   RepositoryEvidence
		wantError bool
	}{
		{"testdata/fixture.bin", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Files: []string{"testdata/fixture.bin"}}}, true},
		{"./testdata//fixture.bin", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Files: []string{"testdata/fixture.bin"}}}, true},
		{"testdata/a/../fixture.bin", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Files: []string{"testdata/fixture.bin"}}}, true},
		{"testdata", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Files: []string{"testdata/fixture.bin"}}}, true},
		{"testdata/with space.bin ", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Files: []string{"testdata/with space.bin "}}}, true},
		{"missing.txt", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head"}}, false},
		{"missing.txt", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Omitted: 1}}, true},
		{"missing.txt", RepositoryEvidence{}, true},
		{"../missing.txt", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head"}}, true},
		{"a/../../missing.txt", RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head"}}, true},
	} {
		t.Run(test.path, func(t *testing.T) {
			provider := &fakeBackend{finalText: `{"decision":"repair","summary":"missing delivery","findings":[{"severity":"major","message":"supply it","absent":"` + test.path + `"}]}`}
			request := newRequest(nil)
			request.Repository = test.listing
			result, err := (Reviewer{Backend: provider, Model: testReviewModel}).Review(context.Background(), request)
			if (err != nil) != test.wantError {
				t.Fatalf("Review() = %#v, %v", result, err)
			}
			if test.wantError && result.Decision != "" {
				t.Fatalf("unsupported claim became a verdict: %#v", result)
			}
		})
	}
}

func TestReviewAbsenceIncludesUncommittedAdditionsAndExcludesDeletions(t *testing.T) {
	t.Parallel()
	evidence := RepositoryEvidence{Listing: gitworktree.CommitListing{Commit: "head", Files: []string{"removed.txt"}}}
	changes := gitworktree.ChangeDiff{Files: []gitworktree.ChangedFile{{Path: "added.txt", Status: "??"}, {Path: "removed.txt", Status: "D"}}}
	for _, path := range []string{"added.txt", "removed.txt"} {
		err := evidence.refute(Verdict{Findings: []Finding{{Absent: path}}}, changes)
		if (err != nil) != (path == "added.txt") {
			t.Fatalf("refute(%q) = %v", path, err)
		}
	}
}

func TestRepositoryEvidenceQuotesWholeContentAndMakesBoundsExplicit(t *testing.T) {
	t.Parallel()
	evidence := RepositoryEvidence{
		Listing:  gitworktree.CommitListing{Commit: "head", Files: []string{"README.md"}},
		Contents: []RepositoryFile{{Path: "README.md", Size: 3200, Content: strings.Repeat("literal\n", 400)}},
	}
	prompt := renderRepository(evidence)
	if !strings.Contains(prompt, "Whole file, 3200 bytes") || strings.Count(prompt, "literal\n") != 400 {
		t.Fatalf("whole content not supplied: %s", prompt)
	}
	bounded := evidence.bounded(800)
	if len(renderRepository(bounded)) > 800 || !strings.Contains(renderRepository(bounded), "Content not supplied") {
		t.Fatalf("input bound did not make the content loss explicit: %s", renderRepository(bounded))
	}
	if evidence.Contents[0].Unavailable != "" {
		t.Fatal("bounding mutated the original evidence")
	}
}

func TestReviewBoundsRepositoryEvidenceAgainstTheWholeInput(t *testing.T) {
	t.Parallel()
	request := newRequest(nil)
	request.Context = strings.Repeat("c", 350<<10)
	request.Changes.Patch = strings.Repeat("p", 350<<10)
	request.Repository.Listing.Commit = "head"
	for index := 0; index < 6000; index++ {
		request.Repository.Listing.Files = append(request.Repository.Listing.Files, fmt.Sprintf("long/path/to/repository/file-%06d.txt", index))
	}
	provider := &fakeBackend{finalText: `{"decision":"approve","approves":"implementation","summary":"all changed code was shown"}`}
	if _, err := (Reviewer{Backend: provider, Model: testReviewModel}).Review(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(provider.request.Prompt)+len(provider.request.SystemPrompt) > MaxReviewInputBytes || !strings.Contains(provider.request.Prompt, "listing omitted") {
		t.Fatal("the whole input did not bound repository evidence explicitly")
	}
	provider.finalText = `{"decision":"repair","summary":"missing","findings":[{"severity":"major","message":"add it","absent":"missing.txt"}]}`
	if result, err := (Reviewer{Backend: provider, Model: testReviewModel}).Review(context.Background(), request); err == nil || result.Decision != "" {
		t.Fatalf("an absence claim was accepted against the bounded listing: %#v, %v", result, err)
	}
}
