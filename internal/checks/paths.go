package checks

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ReadPathPatterns reads the file a path check names, relative to root: the
// paths the check vouches for, one pattern a line.
//
// The patterns are the part of a .gitignore everybody already reads. A blank
// line and a line starting with "#" say nothing. A pattern with no "/" in it
// matches any component of a path, so `*_test.go` is every Go test file and
// `testdata` every testdata directory; one with a "/" is anchored at the
// repository root, so `cmd/yoyo` is that directory and nothing else. A trailing
// "/" limits a pattern to directories, and a directory covers everything below
// it. `*`, `?`, and `[...]` match within one component. A leading "!" takes back
// what an earlier pattern covered, and the last pattern matching a path decides.
func ReadPathPatterns(root, file string) ([]string, error) {
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	var patterns []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for line := 1; scanner.Scan(); line++ {
		pattern := strings.TrimSpace(scanner.Text())
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		body := strings.Trim(strings.TrimPrefix(pattern, "!"), "/")
		if body == "" {
			return nil, fmt.Errorf("%s:%d: %q names no path", file, line, pattern)
		}
		if _, err := path.Match(body, ""); err != nil {
			return nil, fmt.Errorf("%s:%d: %q is not a pattern: %w", file, line, pattern, err)
		}
		patterns = append(patterns, pattern)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	return patterns, nil
}

// Touching reports the first changed path the patterns cover, and whether
// there is one. The paths are repository-relative, as a change names them.
func Touching(patterns, changed []string) (string, bool) {
	for _, changedPath := range changed {
		relative := path.Clean(filepath.ToSlash(strings.TrimSpace(changedPath)))
		if relative == "" || relative == "." || strings.HasPrefix(relative, "../") {
			continue
		}
		if covered(patterns, relative) {
			return relative, true
		}
	}
	return "", false
}

// ChangesFile reports whether a change's paths include the one file named,
// both repository-relative.
func ChangesFile(changed []string, file string) bool {
	want := path.Clean(filepath.ToSlash(strings.TrimSpace(file)))
	for _, changedPath := range changed {
		if path.Clean(filepath.ToSlash(strings.TrimSpace(changedPath))) == want {
			return true
		}
	}
	return false
}

func covered(patterns []string, file string) bool {
	result := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		if matchesPattern(strings.TrimPrefix(pattern, "!"), file) {
			result = !negated
		}
	}
	return result
}

// matchesPattern reports whether one pattern, without its "!", covers a file.
func matchesPattern(pattern, file string) bool {
	directoryOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	anchored := strings.Contains(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	segments := strings.Split(file, "/")
	last := len(segments) - 1
	if anchored {
		// A prefix of the path is a directory the file is under, or the file
		// itself where the prefix is the whole path.
		for index := range segments {
			matched, _ := path.Match(pattern, strings.Join(segments[:index+1], "/"))
			if matched && (index < last || !directoryOnly) {
				return true
			}
		}
		return false
	}
	for index, segment := range segments {
		matched, _ := path.Match(pattern, segment)
		if matched && (index < last || !directoryOnly) {
			return true
		}
	}
	return false
}
