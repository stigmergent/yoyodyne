package config

import (
	"os"
	"strings"
	"testing"
)

// The generated file puts the product block directly under the version, and
// the comment above the specifications directory says every role reads it as
// authoritative.
func TestTheScaffoldPlacesTheProductBlockUnderTheVersionAndSaysEveryRoleReadsIt(t *testing.T) {
	scaffold, err := NewScaffold(BuiltinV1, ScaffoldOptions{ProductID: "example", Repository: "."})
	if err != nil {
		t.Fatalf("NewScaffold() error = %v", err)
	}
	content := string(scaffold.Config.Content)
	var keys []string
	for _, line := range strings.Split(content, "\n") {
		if match := topLevelKeyPattern.FindStringSubmatch(line); match != nil {
			keys = append(keys, match[1])
		}
	}
	if len(keys) < 2 || keys[0] != "version" || keys[1] != "product" {
		t.Fatalf("top-level keys open %v, want version and then product", keys)
	}
	comment, found := commentAbove(content, "product", "specifications")
	if !found {
		t.Fatal("the generated file has no product.specifications")
	}
	if joinComment(comment) != joinComment(specificationsComment) {
		t.Fatalf("comment above product.specifications = %q", joinComment(comment))
	}
	if !strings.Contains(joinComment(comment), "Every role reads the documents under this directory as authoritative") {
		t.Fatalf("the comment does not say every role reads the directory as authoritative: %q", joinComment(comment))
	}
}

// A copy carrying a wording the template wrote earlier is offered the current
// one; a copy somebody rewrote, or one with no comment, is theirs and is never
// offered anything; and a copy that says what the template says is unchanged.
func TestTheSpecificationsCommentIsComparedAgainstEveryWordingTheTemplateShipped(t *testing.T) {
	file := func(comment []string) string {
		var rendered strings.Builder
		rendered.WriteString("version: 1\n\nproduct:\n  id: example\n  repository: .\n")
		for _, line := range comment {
			rendered.WriteString("  # " + line + "\n")
		}
		rendered.WriteString("  specifications: docs/product\n  designs: docs/designs\n\nexecution:\n  # specifications: not this one\n  max_concurrent_developers: 1\n")
		return rendered.String()
	}
	classOf := func(comment []string) Class {
		yours, found := commentAbove(file(comment), "product", "specifications")
		if !found {
			t.Fatal("product.specifications not found")
		}
		return compareComment(SpecificationsCommentKey, yours, specificationsComment, earlierSpecificationsComments).Class
	}
	if class := classOf(specificationsComment); class != ClassUnchanged {
		t.Fatalf("the current wording reads as %s, want unchanged", class)
	}
	for index, earlier := range earlierSpecificationsComments {
		if class := classOf(earlier); class != ClassAvailable {
			t.Fatalf("earlier wording %d reads as %s, want available", index, class)
		}
	}
	if class := classOf([]string{"Our own note about this directory."}); class != ClassYours {
		t.Fatalf("a rewritten comment reads as %s, want yours", class)
	}
	if class := classOf(nil); class != ClassYours {
		t.Fatalf("a deleted comment reads as %s, want yours", class)
	}
	// Rewrapping the same words is not a rewrite.
	rewrapped := strings.Fields(strings.Join(earlierSpecificationsComments[0], " "))
	if class := classOf([]string{strings.Join(rewrapped[:10], " "), strings.Join(rewrapped[10:], " ")}); class != ClassAvailable {
		t.Fatalf("a rewrapped earlier wording reads as %s, want available", class)
	}
}

// The comparison needs no baseline: a project with no config.lock is still told.
func TestCommentDriftIsReadWithoutABaseline(t *testing.T) {
	resolved := loadScaffoldEdited(t, ScaffoldOptions{ProductID: "example", Repository: "."}, func(content string) string {
		return strings.Replace(content, renderSpecificationsComment(),
			"  # "+strings.Join(earlierSpecificationsComments[2], "\n  # ")+"\n", 1)
	})
	if err := os.Remove(LockPath(resolved.Path)); err != nil {
		t.Fatalf("remove the baseline: %v", err)
	}
	drift, unknown := ReadDrift(resolved)
	if drift.Known || !unknown.Absent {
		t.Fatalf("drift = %+v, unknown = %+v; want no baseline", drift, unknown)
	}
	if len(drift.Comments) != 1 || drift.Comments[0].Key != SpecificationsCommentKey || drift.Comments[0].Class != ClassAvailable {
		t.Fatalf("comments = %+v, want the specifications comment available", drift.Comments)
	}
}
