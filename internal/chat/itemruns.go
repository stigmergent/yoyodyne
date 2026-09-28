package chat

// The runs made for a work item, carried beside the item when a role reads it.
//
// A read of an item is bounded, and what it cuts is the front of the notes, so
// the end — what was written most recently — is kept. The run a stoppage is about
// is named in the notes by whatever wrote the stoppage down, and that writing is
// often not the most recent: on 2026-09-26 the development manager could not find
// the run to record a decision on for yoyodyne-ifd.430.13.4, because the read had
// cut the note that named it, and the operator's assistant had to hand her the
// identifier. A decision names a run, so a role that cannot find the run cannot
// decide.
//
// So the runs are read from the harness's own records rather than from the notes,
// and said in a section of their own after the item. The section is bounded by
// how many runs it lists rather than cut with the notes, and the run the item's
// hold is about is listed whatever else is, because that is the one a decision
// has to name.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// maxItemRunsListed bounds how many runs one read of an item lists. An item
// rarely has more than a handful, and one that has more is one whose newest runs
// are the ones anybody is deciding about; the count of the rest is still said.
const maxItemRunsListed = 10

// maxItemHoldReasonBytes bounds the sentence saying what holds the item. The
// reasons the harness writes are a few hundred bytes; the bound is there so a
// reason nobody expected cannot become the read.
const maxItemHoldReasonBytes = 2 * maxTrackerFailureBytes

// renderItemRuns is the runs section of one item's read: every run the harness
// recorded for it, newest first, with when each started and ended, what became
// of it, and whether its change is still there; and which run the item's
// current hold is about, if anything holds it.
//
// A record that could not be read is said as unread rather than left out: a
// section that fell back to silence would read as an item nothing was ever run
// for, which is the one reading that sends nobody looking.
func (s *Session) renderItemRuns(ctx context.Context, workItemID string) string {
	if s.options.Work == nil {
		return s.renderItemRunsFrom(ctx, workItemID, ItemPrice{}, nil)
	}
	price, err := s.options.Work.Price(ctx, workItemID)
	return s.renderItemRunsFrom(ctx, workItemID, price, err)
}

// renderItemRunsFrom is the runs section from a price already read, which is
// how `/show` says both the runs and what they cost from one reading of the
// records.
func (s *Session) renderItemRunsFrom(ctx context.Context, workItemID string, price ItemPrice, err error) string {
	var rendered strings.Builder
	holdRunID, holdLine := s.itemHold(ctx, workItemID)
	if s.options.Work == nil {
		rendered.WriteString("\nruns: this conversation cannot read the harness's run records, so the runs made for this item are unknown rather than none.\n")
		rendered.WriteString(holdLine)
		return rendered.String()
	}
	if err != nil {
		fmt.Fprintf(&rendered, "\nruns: could not be read, so treat them as unknown rather than none: %s\n",
			singleLine(err.Error(), maxTrackerFailureBytes))
		rendered.WriteString(holdLine)
		return rendered.String()
	}
	if len(price.Runs) == 0 {
		rendered.WriteString("\nruns: the harness has no recorded run of this item.\n")
		rendered.WriteString(holdLine)
		return rendered.String()
	}
	listed, unlisted := listedItemRuns(price.Runs, holdRunID)
	fmt.Fprintf(&rendered, "\nruns (%d recorded, newest first):\n", len(price.Runs))
	for _, run := range listed {
		fmt.Fprintf(&rendered, "- %s\n", describeItemRun(run, run.RunID == holdRunID))
	}
	if unlisted > 0 {
		fmt.Fprintf(&rendered, "%d older run(s) are not listed here.\n", unlisted)
	}
	if holdRunID != "" && !runRecorded(price.Runs, holdRunID) {
		fmt.Fprintf(&rendered, "The hold below is about run %s, which the harness has no record of among this item's runs.\n", holdRunID)
	}
	rendered.WriteString(holdLine)
	return rendered.String()
}

// itemHold is the run the item's current hold is about, and the line saying
// what holds it. The line is said whether or not anything holds the item, since
// "nothing holds it" and "what holds it could not be read" are opposite answers
// to whether there is a stoppage to decide about.
func (s *Session) itemHold(ctx context.Context, workItemID string) (string, string) {
	if s.options.Held == nil {
		return "", "held: what the harness is holding back after stopped runs cannot be read from this conversation, so whether a stoppage holds this item is unknown.\n"
	}
	holds, err := s.options.Held.HeldForAPerson(ctx)
	if err != nil {
		return "", fmt.Sprintf("held: what the harness is holding back after stopped runs could not be read, so whether a stoppage holds this item is unknown: %s\n",
			singleLine(err.Error(), maxTrackerFailureBytes))
	}
	reason, held := holds.Reason(workItemID)
	if !held {
		return "", "held: nothing the harness records after a stopped run is holding this item.\n"
	}
	runID := holds.RunID(workItemID)
	about := "the record holding it names no run"
	if runID != "" {
		about = "it is about run " + runID
	}
	return runID, fmt.Sprintf("held: %s; %s.\n", about, singleLine(reason, maxItemHoldReasonBytes))
}

// listedItemRuns is the runs a read lists, newest first, and how many it leaves
// out. The run the hold is about is always listed, in place of the oldest the
// bound would otherwise have kept, because it is the one a decision names.
func listedItemRuns(runs []RunPrice, holdRunID string) ([]RunPrice, int) {
	newest := make([]RunPrice, 0, len(runs))
	for index := len(runs) - 1; index >= 0; index-- {
		newest = append(newest, runs[index])
	}
	if len(newest) <= maxItemRunsListed {
		return newest, 0
	}
	listed := append([]RunPrice(nil), newest[:maxItemRunsListed]...)
	if holdRunID != "" && !runRecorded(listed, holdRunID) {
		for _, run := range newest[maxItemRunsListed:] {
			if run.RunID == holdRunID {
				listed[len(listed)-1] = run
				break
			}
		}
	}
	return listed, len(runs) - len(listed)
}

func runRecorded(runs []RunPrice, runID string) bool {
	for _, run := range runs {
		if run.RunID == runID {
			return true
		}
	}
	return false
}

// describeItemRun is one run on one line: its identifier, when it started and
// ended, what became of it, and whether its change is still there.
func describeItemRun(run RunPrice, holdIsAboutIt bool) string {
	ended := "not ended"
	if run.CompletedAt != nil && !run.CompletedAt.IsZero() {
		ended = "ended " + run.CompletedAt.UTC().Format(time.RFC3339)
	}
	remains := run.Remains
	if remains == "" {
		remains = "what survives of its change not recorded"
	}
	line := fmt.Sprintf("%s started %s, %s [%s] %s", run.RunID, run.StartedAt.UTC().Format(time.RFC3339), ended, run.outcome(), remains)
	if holdIsAboutIt {
		line += " — the item's hold is about this run"
	}
	return line
}
