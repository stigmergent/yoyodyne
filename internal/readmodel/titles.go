package readmodel

// A work item's title beside its number, wherever a person reads one.
//
// On 2026-09-26 the operator read a program manager's lane report naming work
// as "434.9 and 434.3", with nothing saying what either was. The roles are told
// to name an item by what it is, and that is not enough on its own: a surface
// that prints whatever a role wrote lets the next bare number through. So every
// surface that renders text for a person — the four lines and the needs-a-human
// line under them, a lane report and the card it is opened on, a pass's account
// in `yoyo sweeps`, the report pile a digest is filed into, and every message
// the channel posts — passes that text through Cite, and a number reaches the
// reader with the item's title beside it.
//
// Three shapes of identifier are read, and they are not treated alike, because
// only one of them is unmistakably an identifier:
//
//   - The full form, "yoyodyne-ifd.434.9": the tracker's own prefix, a hash,
//     and the dotted children. Nothing else in prose has that shape, so one the
//     tracker does not hold is said to be unknown rather than left bare.
//   - The form without the product, "ifd.434.9": the same, where exactly one
//     root in the tracker carries that hash.
//   - The bare dotted number, "434.9", which is what the lane report said. A
//     dotted number is also a version, a decimal, and an address, so it is read
//     as an item only where the tracker holds exactly one item it could name,
//     and is left alone otherwise: calling every "3.5" an unknown work item
//     would be a surface wrong in a new way.
//
// An identifier whose title the text already says is left as it is, and so is
// every later mention of one already titled, so a line that was written well is
// not written twice and a paragraph naming one item five times titles it once.

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mason-bryant/yoyodyne/internal/beads"
)

// maxCitedTitleBytes bounds the title put beside a number. Titles in this
// tracker run to a paragraph, and the point is to say what the item is, not to
// quote it whole into a status line; the item's own card has the rest.
const maxCitedTitleBytes = 100

// titleEchoRunes is how much of a title has to appear in the text for the text
// to be taken as already saying it. A prefix rather than the whole, because a
// thread header or a listing may already carry the title cut to its own bound.
const titleEchoRunes = 40

// unknownWorkItem is what an identifier the tracker holds nothing under is shown
// with, in place of a title. Dropping it would lose what the role wrote; leaving
// it bare is the defect this exists to end.
const unknownWorkItem = "unknown to the tracker"

// untitledWorkItem is what an item the tracker holds with no title is shown
// with, which is a different fact from the tracker not holding it.
const untitledWorkItem = "no title recorded"

// candidateToken is a run of the characters an identifier is made of. Each one
// is then classified; most are ordinary words and are passed over.
var candidateToken = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._-]*`)

var (
	dottedNumber = regexp.MustCompile(`^[0-9]+(\.[0-9]+)+$`)
	children     = regexp.MustCompile(`^(\.[0-9]+)*$`)
)

// quantityUnits are the words a dotted number is a quantity of rather than an
// item, when one follows it: "3.5 hours" is a duration whether or not the
// tracker holds a 3.5.
var quantityUnits = map[string]bool{
	"s": true, "ms": true, "sec": true, "secs": true, "second": true, "seconds": true,
	"m": true, "min": true, "mins": true, "minute": true, "minutes": true,
	"h": true, "hr": true, "hrs": true, "hour": true, "hours": true,
	"d": true, "day": true, "days": true, "week": true, "weeks": true,
	"kb": true, "mb": true, "gb": true, "kib": true, "mib": true, "gib": true, "tokens": true,
	"percent": true, "per": true, "times": true, "x": true, "dollars": true, "usd": true,
}

// WorkItemTitles is every item the tracker holds, by identifier, as the read
// model resolves the numbers a person reads. The zero value and nil both know
// nothing, and Cite on either leaves text exactly as it was: a tracker that
// could not be read is not a tracker that holds no items, and saying "unknown"
// over every number because the listing failed would be false.
type WorkItemTitles struct {
	titles map[string]string
	// roots is each item's identifier up to its first child, "yoyodyne-ifd",
	// which is what a bare number is read against.
	roots map[string]bool
	// prefixes is each root without its hash, "yoyodyne", which is what makes a
	// full identifier recognisable as one the tracker does not hold.
	prefixes map[string]bool
	// hashes is each root's hash, "ifd", with the roots carrying it.
	hashes map[string][]string
}

// NewWorkItemTitles indexes a listing of the tracker's items.
func NewWorkItemTitles(items []beads.WorkItem) *WorkItemTitles {
	index := &WorkItemTitles{
		titles:   make(map[string]string, len(items)),
		roots:    map[string]bool{},
		prefixes: map[string]bool{},
		hashes:   map[string][]string{},
	}
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		index.titles[id] = strings.TrimSpace(item.Title)
		root, _, _ := strings.Cut(id, ".")
		if index.roots[root] {
			continue
		}
		index.roots[root] = true
		if cut := strings.LastIndex(root, "-"); cut > 0 && cut < len(root)-1 {
			index.prefixes[root[:cut]] = true
			hash := root[cut+1:]
			index.hashes[hash] = append(index.hashes[hash], root)
		}
	}
	return index
}

// ReadWorkItemTitles lists every item the tracker holds, closed work included,
// for the titles. It is one tracker command however many numbers are resolved.
func ReadWorkItemTitles(ctx context.Context, sources Sources) (*WorkItemTitles, error) {
	if sources.Tracker == nil {
		return nil, nil
	}
	items, err := sources.list(ctx, "")
	if err != nil {
		return nil, err
	}
	return NewWorkItemTitles(items), nil
}

// Title is what the tracker calls one item, and whether it holds the item.
func (w *WorkItemTitles) Title(id string) (string, bool) {
	if w == nil {
		return "", false
	}
	title, known := w.titles[id]
	return title, known
}

// Cite is text with every work item identifier in it shown beside the item's
// title, or beside the word that the tracker does not know it.
func (w *WorkItemTitles) Cite(text string) string {
	return w.CiteAfter("", text)
}

// CiteAfter is Cite for text that follows something the reader has just read:
// an item whose title that already said is not titled again. It is what puts
// two halves of one line, written separately, through as the one line they are
// read as.
func (w *WorkItemTitles) CiteAfter(prior, text string) string {
	if w == nil || len(w.titles) == 0 || text == "" {
		return text
	}
	matches := candidateToken.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var cited strings.Builder
	said := folded(prior + " " + text)
	last := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		// A sentence's closing stop and a hyphen dangling off the end are not
		// part of the identifier.
		for end > start && (text[end-1] == '.' || text[end-1] == '-' || text[end-1] == '_') {
			end--
		}
		token := text[start:end]
		id, definite := w.identify(token)
		if id == "" || !w.standsAlone(text, start, end) || insideCode(text, start) {
			continue
		}
		title, known := w.titles[id]
		if !known && !definite {
			continue
		}
		var beside string
		switch {
		case !known:
			beside = unknownWorkItem
		case title == "":
			beside = untitledWorkItem
		default:
			if alreadySaid(said, text, start, end, title) {
				continue
			}
			beside = singleLine(title, maxCitedTitleBytes)
		}
		cited.WriteString(text[last:end])
		cited.WriteString(" (" + beside + ")")
		last = end
		// What has been said now includes the title, so a later mention of the
		// same item in the same text is left as the reader already has it.
		said += " " + folded(beside)
	}
	if last == 0 {
		return text
	}
	cited.WriteString(text[last:])
	return cited.String()
}

// identify reads one token as a work item identifier: the identifier it names,
// and whether its shape alone makes it one — so that one the tracker does not
// hold is said to be unknown — rather than only a number that happens to name
// an item.
func (w *WorkItemTitles) identify(token string) (string, bool) {
	if _, known := w.titles[token]; known {
		return token, true
	}
	root, rest, _ := strings.Cut(token, ".")
	if rest != "" {
		rest = "." + rest
	}
	if !children.MatchString(rest) {
		return "", false
	}
	// The full form: a prefix the tracker uses, a hash, and children. A root with
	// no children the tracker does not hold is a hyphenated word far more often
	// than a missing epic, so it takes a child to be said to be unknown.
	if cut := strings.LastIndex(root, "-"); cut > 0 && w.prefixes[root[:cut]] && isHash(root[cut+1:]) {
		return token, rest != ""
	}
	// The form without the product: a hash exactly one root carries.
	if rest != "" {
		if roots := w.hashes[root]; len(roots) == 1 {
			return roots[0] + rest, true
		}
	}
	// The bare dotted number, read only where exactly one item answers to it.
	if dottedNumber.MatchString(token) {
		var found string
		for candidate := range w.roots {
			if _, known := w.titles[candidate+"."+token]; known {
				if found != "" {
					return "", false
				}
				found = candidate + "." + token
			}
		}
		return found, false
	}
	return "", false
}

// standsAlone reports whether the token at [start, end) is a word of its own
// rather than part of a path, a link, an amount, or a quantity.
func (w *WorkItemTitles) standsAlone(text string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if strings.ContainsRune(`/=#$<@\~≥≤+`, before) {
			return false
		}
	}
	if end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if strings.ContainsRune(`/%=@>`, after) {
			return false
		}
		// Something already said beside it, by an earlier reading of the same
		// text, is not said again.
		rest := text[end:]
		if strings.HasPrefix(rest, " ("+unknownWorkItem+")") || strings.HasPrefix(rest, " ("+untitledWorkItem+")") {
			return false
		}
	}
	if dottedNumber.MatchString(text[start:end]) {
		following := strings.Fields(text[end:])
		if len(following) > 0 {
			unit := strings.ToLower(strings.TrimRight(following[0], ".,;:)"))
			if quantityUnits[unit] {
				return false
			}
		}
	}
	return true
}

// insideCode reports whether the offset falls inside an inline code span on its
// line. What is between backticks is something to be typed, and a title put
// into the middle of a command is a command that no longer runs.
func insideCode(text string, offset int) bool {
	lineStart := strings.LastIndexByte(text[:offset], '\n') + 1
	return strings.Count(text[lineStart:offset], "`")%2 == 1
}

// isHash reports whether a segment has the shape of a tracker hash: short,
// lower-case letters and digits.
func isHash(segment string) bool {
	if segment == "" || len(segment) > 12 {
		return false
	}
	for _, r := range segment {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// shortEchoRunes is the length under which a title is too common a phrase to
// be taken as said wherever it occurs: "Fix the build" anywhere in a paragraph
// says nothing about which item a number beside some other sentence is. A
// title that short counts as said only where it is next to the number.
const shortEchoRunes = 16

// nearbyBytes is how far from a number "next to it" reaches, either side.
const nearbyBytes = 80

// alreadySaid reports whether the text already names the item at [start, end)
// by its title.
func alreadySaid(said, text string, start, end int, title string) bool {
	echo := echoOf(title)
	if utf8.RuneCountInString(echo) >= shortEchoRunes {
		return strings.Contains(said, echo)
	}
	from, to := start-nearbyBytes, end+nearbyBytes
	if from < 0 {
		from = 0
	}
	if to > len(text) {
		to = len(text)
	}
	return strings.Contains(folded(text[from:to]), echo)
}

// folded is text lower-cased with its whitespace run together, which is how a
// title is looked for in it: a title a line wrapped is still a title said.
func folded(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// echoOf is the part of a title whose presence in the text says the text
// already names the item, folded as the text it is looked for in is.
func echoOf(title string) string {
	lowered := folded(title)
	if utf8.RuneCountInString(lowered) <= titleEchoRunes {
		return lowered
	}
	runes := []rune(lowered)
	return string(runes[:titleEchoRunes])
}
