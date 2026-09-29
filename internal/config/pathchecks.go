package config

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// PathCheck is one check the per-run gate runs only for a change that touches
// what it vouches for: a walkthrough of the documented install, say, which a
// change to the documentation or to the program it documents can break and a
// change to anything else cannot.
type PathCheck struct {
	// Command is the check, run through "/bin/sh -c" in the run's worktree like
	// every entry in Checks.
	Command string `yaml:"command" json:"command"`
	// Paths is the repository-relative file listing the paths the check vouches
	// for, one pattern a line. It is read from the worktree of the change under
	// test, so a change that widens what the check covers is judged by the
	// widened list.
	Paths string `yaml:"paths" json:"paths"`
}

func (c PathCheck) problems(index int) []string {
	var problems []string
	if strings.TrimSpace(c.Command) == "" {
		problems = append(problems, fmt.Sprintf("path check %d: command cannot be empty", index))
	}
	paths := strings.TrimSpace(c.Paths)
	switch {
	case paths == "":
		problems = append(problems, fmt.Sprintf("path check %d: paths must name the file listing what the check vouches for", index))
	case filepath.IsAbs(paths) || path.IsAbs(filepath.ToSlash(paths)):
		problems = append(problems, fmt.Sprintf("path check %d: paths %q must be relative to the repository", index, c.Paths))
	default:
		cleaned := path.Clean(filepath.ToSlash(paths))
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			problems = append(problems, fmt.Sprintf("path check %d: paths %q must name a file inside the repository", index, c.Paths))
		}
	}
	return problems
}
