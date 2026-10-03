package orchestrator

// Reading the configuration a landing left, for the comparison against the
// running parts of the product that nameUnreadingParts makes.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
)

// maxLandedConfigBytes bounds the configuration a landing reads at its commit,
// the bound every state record here is held to.
const maxLandedConfigBytes = 1 << 20

// configAtCommit reads a configuration file a running part reads as commit
// holds it, where the file is inside the repository and the commit carries
// it, and as it stands on disk otherwise.
func (p *Pipeline) configAtCommit(ctx context.Context, commit, configPath string) ([]byte, error) {
	relative, inside := repositoryRelative(p.Repository, configPath)
	if !inside || strings.TrimSpace(commit) == "" || p.Worktrees == nil {
		return os.ReadFile(configPath)
	}
	file, err := p.Worktrees.FileAtCommit(ctx, commit, relative, maxLandedConfigBytes)
	if errors.Is(err, gitworktree.ErrNotAtCommit) {
		return os.ReadFile(configPath)
	}
	if err != nil {
		return nil, err
	}
	if file.Content == nil && file.Size > 0 {
		return nil, fmt.Errorf("%s at %s is %d bytes, past the %d byte bound", relative, commit, file.Size, maxLandedConfigBytes)
	}
	return file.Content, nil
}

// repositoryRelative is path within repository in slash form, reporting
// whether it is inside it at all. Both are resolved through their symlinks
// where they can be, because a recorded path and the pipeline's root can name
// one directory two ways — /var and /private/var on macOS.
func repositoryRelative(repository, path string) (string, bool) {
	if strings.TrimSpace(repository) == "" {
		return "", false
	}
	for _, pair := range [][2]string{{repository, path}, {resolved(repository), resolved(path)}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return filepath.ToSlash(relative), true
		}
	}
	return "", false
}

func resolved(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}
