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

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// maxLandedConfigBytes bounds the configuration a landing reads at its commit,
// the bound every state record here is held to.
const maxLandedConfigBytes = 1 << 20

// templateConfigMismatches compares keys added to shipped templates at this
// landing, independently of the active files. Both versions come from Git:
// the checkout and the checker's embedded template can each be older.
func (a *activeRun) templateConfigMismatches(ctx context.Context) ([]runstate.ConfigTemplateMismatch, error) {
	p := a.pipeline
	var mismatches []runstate.ConfigTemplateMismatch
	var problems []error
	for _, path := range config.BuiltinTemplatePaths() {
		after, err := p.templateAtCommit(ctx, a.outcome.Integration.TargetCommit, path)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if after == nil {
			continue
		}
		before, err := p.templateAtCommit(ctx, a.outcome.Integration.PreviousTargetCommit, path)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		added, err := config.AddedTemplateKeys(before, after)
		if err != nil {
			problems = append(problems, fmt.Errorf("compare shipped template %s: %w", path, err))
			continue
		}
		if len(added) == 0 {
			continue
		}
		found, err := p.ConfigReaders.TemplateMismatches(path, added)
		mismatches = append(mismatches, found...)
		problems = append(problems, err)
	}
	return mismatches, errors.Join(problems...)
}

func (p *Pipeline) templateAtCommit(ctx context.Context, commit, path string) ([]byte, error) {
	if strings.TrimSpace(commit) == "" || p.Worktrees == nil {
		return nil, fmt.Errorf("read shipped template %s: the landing names no commit or repository reader", path)
	}
	file, err := p.Worktrees.FileAtCommit(ctx, commit, path, maxLandedConfigBytes)
	if errors.Is(err, gitworktree.ErrNotAtCommit) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read shipped template %s at %s: %w", path, commit, err)
	}
	if file.Content == nil && file.Size > 0 {
		return nil, fmt.Errorf("%s at %s is %d bytes, past the %d byte bound", path, commit, file.Size, maxLandedConfigBytes)
	}
	return file.Content, nil
}

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
