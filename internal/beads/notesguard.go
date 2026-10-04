package beads

// The writer that replaces earlier notes, read in a command line before it
// runs rather than found in the wreckage afterwards.
//
// The client preserves earlier notes: Update passes --append-notes, and
// Create passes --notes onto an item that does not exist yet. Recorded losses
// came from outside those paths -- `bd update <id> --notes="..."` typed into an
// agent session, twelve times across twelve items, each replacement taking the
// `Goal served:` line with it and leaving the item reading as work nobody ever
// attributed. The flag is the mechanism and the sessions are the writer, so
// this is the flag recognised where the session is still able to be stopped.
//
// Notes are append-only: a correction adds a new note and preserves every
// earlier one. Carrying a goal attribution through a replacement does not
// preserve the rest of the record, so every recognised replacement is refused.
//
// The rule is decidable from the command line alone, deliberately. This runs in
// front of every shell command an agent takes, and a guard that asked the
// tracker what the item currently records would be a guard that waits on a
// locked database on every command. Recognising the replacement flag is enough
// to refuse it, regardless of what the item records.
// The refusal directs the writer to append instead, without reading the item
// or offering a replacement escape.

import (
	"fmt"
	"path"
	"strings"
)

// notesFlag replaces an item's notes wholesale, and appendNotesFlag adds to
// them. Both spellings are named here because the refusal is about the
// difference between them: the second is what a refusal sends the writer to,
// and it must not be read as the first.
const (
	notesFlag       = "--notes"
	appendNotesFlag = "--append-notes"
)

// DestroyedAttribution says why a shell command line must not run: some command
// in it replaces a work item's notes wholesale, contrary to the append-only
// rule. A goal line in the replacement does not preserve every earlier note.
// It is empty for every other command line, which is nearly all of them.
//
// The first such command decides the answer. A line carrying two of them is one
// refusal to act on, not two, and the writer meets the second only after
// rewriting the first.
func DestroyedAttribution(command string) string {
	for _, words := range simpleCommands(command) {
		replaced, replaces := notesReplacement(words)
		if !replaces {
			continue
		}
		return fmt.Sprintf("`bd update %s %s` replaces that item's notes wholesale. "+
			"A work item's notes are append-only: nothing rewrites, reorders, or removes an earlier note. "+
			"Use `%s` instead, including for corrections: it adds a new note and takes nothing away. "+
			"The replacement is refused even if it includes a `Goal served:` line.",
			replaced, notesFlag, appendNotesFlag)
	}
	return ""
}

// notesReplacement reads one command's words as a wholesale notes replacement:
// the item whose notes it replaces and whether it is one at all. The
// replacement's contents do not decide whether it is refused.
//
// The item is what the command names first that is not a flag, and it is only
// ever used to say which item is at stake. A command that names none -- or
// whose first bare word belongs to some other flag -- is still refused, because
// what decides that is the replacement rather than the naming.
func notesReplacement(words []string) (string, bool) {
	if len(words) < 2 || !invokesBd(words[0]) || words[1] != "update" {
		return "", false
	}
	replaced, replaces := "", false
	for index := 2; index < len(words); index++ {
		word := words[index]
		switch {
		case word == notesFlag:
			// The replacement written as two words. A trailing `--notes` with
			// nothing after it replaces the notes with nothing, which is the same
			// destruction spelled shorter.
			replaces = true
			if index+1 < len(words) {
				index++
			}
		case strings.HasPrefix(word, notesFlag+"="):
			replaces = true
		case strings.HasPrefix(word, "-"):
			// Every other flag, `--append-notes` among them, which is the whole
			// point: it is not this one.
		case replaced == "":
			replaced = word
		}
	}
	if replaced == "" {
		replaced = "<id>"
	}
	return replaced, replaces
}

// invokesBd reports a word that runs the tracker, however it was reached: bare
// on the PATH, or by a path to it.
func invokesBd(word string) bool {
	return path.Base(strings.ReplaceAll(word, `\`, "/")) == "bd"
}

// simpleCommands splits a shell command line into the words of each command in
// it. Reading only the first command would miss `cd repo && bd update ...`,
// which is how these were actually written.
//
// It is a reading of the line rather than a shell. Quoting is honoured, because
// the replacement text is always quoted and a reading that ignored quotes would
// see a sentence as a hundred flags; substitutions are not expanded and control
// structures are not understood, because neither is how a routine writer types
// this. That bound is the honest one for what this is: it catches the command
// somebody meant to run, and a command line assembled so this cannot see it is
// not the accident it exists to prevent.
func simpleCommands(command string) [][]string {
	var commands [][]string
	var words []string
	var word strings.Builder
	// started tells an empty word that was quoted -- `--notes=""` -- from no
	// word at all, which is the difference between replacing the notes with
	// nothing and not replacing them.
	started := false
	quote := rune(0)
	flush := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	endCommand := func() {
		flush()
		if len(words) > 0 {
			commands = append(commands, words)
			words = nil
		}
	}
	runes := []rune(command)
	for index := 0; index < len(runes); index++ {
		character := runes[index]
		switch {
		case quote == '\'':
			// Single quotes are literal, backslash included, so the only thing
			// that ends them is the closing quote.
			if character == '\'' {
				quote = 0
				continue
			}
			word.WriteRune(character)
			started = true
		case quote == '"':
			if character == '"' {
				quote = 0
				continue
			}
			if character == '\\' && index+1 < len(runes) && strings.ContainsRune("\"\\$`\n", runes[index+1]) {
				index++
				if runes[index] == '\n' {
					continue
				}
				word.WriteRune(runes[index])
				started = true
				continue
			}
			word.WriteRune(character)
			started = true
		case character == '\'' || character == '"':
			quote = character
			started = true
		case character == '\\':
			if index+1 < len(runes) {
				index++
				if runes[index] == '\n' {
					continue
				}
				word.WriteRune(runes[index])
				started = true
			}
		case strings.ContainsRune(";&|\n()", character):
			endCommand()
		case character == ' ' || character == '\t' || character == '\r':
			flush()
		default:
			word.WriteRune(character)
			started = true
		}
	}
	endCommand()
	return commands
}
