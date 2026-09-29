package runstate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

// StateHomeVariable is the explicit instruction that moves the state root for
// one shell, and it wins over everything else because it is one.
const StateHomeVariable = "YOYODYNE_STATE_HOME"

// MachineFileName is the machine-local configuration file, kept in the
// configurations home beside the external project configurations. It describes
// this machine rather than any project, which is why the state root is set here
// and never in a project file: a project file is committed and read on every
// machine that checks the project out, and one that sets the key is refused.
const MachineFileName = config.MachineFileName

// The origins a state root is reported under, in the order they are consulted.
// `yoyo config show --origins` and `yoyo doctor` print them, so they are named
// in the vocabulary an operator would type to change the value.
const (
	RootOriginEnvironment     = "environment:" + StateHomeVariable
	rootOriginMachinePrefix   = "machine:"
	RootOriginXDG             = "environment:XDG_STATE_HOME"
	RootOriginPlatformDefault = "platform-default"
)

// ResolvedRoot is the state root one process resolved and the layer it came
// from.
type ResolvedRoot struct {
	Path string
	// Origin names the layer: one of the RootOrigin constants, or "machine:"
	// followed by the machine file that set it.
	Origin string
}

// machineDocument is the whole of what the machine file may say. It is decoded
// strictly, so a misspelled key is refused rather than leaving the root where it
// was with nothing to say why.
type machineDocument struct {
	StateRoot string `yaml:"state_root"`
}

// MachinePath is where this machine's configuration file is, whether or not it
// exists.
func MachinePath(getenv func(string) string, userHomeDir func() (string, error)) (string, error) {
	home, err := config.ExternalHome(getenv, userHomeDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, MachineFileName), nil
}

// machineStateRoot is the state root the machine file sets, or nothing where
// there is no file or it sets none.
func machineStateRoot(getenv func(string) string, userHomeDir func() (string, error)) (string, string, error) {
	path, err := MachinePath(getenv, userHomeDir)
	if err != nil {
		return "", "", err
	}
	source, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", path, nil
	}
	if err != nil {
		return "", path, fmt.Errorf("read the machine configuration %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	decoder.KnownFields(true)
	var document machineDocument
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return "", path, nil
		}
		return "", path, fmt.Errorf("read the machine configuration %s: %w", path, err)
	}
	value := strings.TrimSpace(document.StateRoot)
	if value == "" {
		return "", path, nil
	}
	if !filepath.IsAbs(value) {
		return "", path, fmt.Errorf("state_root in %s must be an absolute path and is %q", path, value)
	}
	return filepath.Clean(value), path, nil
}

// ResolveRoot is the one resolution of the state root every process makes:
// YOYODYNE_STATE_HOME, then state_root in the machine file, then
// XDG_STATE_HOME/yoyodyne, then the platform default, in that order. The
// variable wins because it is an explicit instruction for this shell; the
// machine key is the operator's standing answer for the machine; the last two
// are what a machine nobody configured gets.
//
// It resolves and never guards. A process that opens a product's records under
// the root agrees it with the checkout's marker first, through AgreeRoot, which
// is what the command package's productStateRoot does; only the two surfaces that
// report the root without opening anything under it — `yoyo config show` and
// `yoyo doctor` — call this without that. TestNothingOpensTheStateRootUnguarded
// in the command package holds every caller to that list.
func ResolveRoot(getenv func(string) string, userHomeDir func() (string, error), goos string) (ResolvedRoot, error) {
	if value := strings.TrimSpace(getenv(StateHomeVariable)); value != "" {
		if !filepath.IsAbs(value) {
			return ResolvedRoot{}, errors.New("YOYODYNE_STATE_HOME must be an absolute path")
		}
		return ResolvedRoot{Path: filepath.Clean(value), Origin: RootOriginEnvironment}, nil
	}
	machine, machinePath, err := machineStateRoot(getenv, userHomeDir)
	if err != nil {
		return ResolvedRoot{}, err
	}
	if machine != "" {
		return ResolvedRoot{Path: machine, Origin: rootOriginMachinePrefix + machinePath}, nil
	}
	if value := strings.TrimSpace(getenv("XDG_STATE_HOME")); value != "" {
		if !filepath.IsAbs(value) {
			return ResolvedRoot{}, errors.New("XDG_STATE_HOME must be an absolute path")
		}
		return ResolvedRoot{Path: filepath.Join(filepath.Clean(value), "yoyodyne"), Origin: RootOriginXDG}, nil
	}

	home, err := userHomeDir()
	if err != nil {
		return ResolvedRoot{}, fmt.Errorf("resolve user home directory: %w", err)
	}
	var path string
	switch goos {
	case "darwin":
		path = filepath.Join(home, "Library", "Application Support", "Yoyodyne", "state")
	case "windows":
		if localAppData := strings.TrimSpace(getenv("LOCALAPPDATA")); localAppData != "" {
			if !filepath.IsAbs(localAppData) {
				return ResolvedRoot{}, errors.New("LOCALAPPDATA must be an absolute path")
			}
			path = filepath.Join(localAppData, "Yoyodyne", "state")
		} else {
			path = filepath.Join(home, "AppData", "Local", "Yoyodyne", "state")
		}
	default:
		path = filepath.Join(home, ".local", "state", "yoyodyne")
	}
	return ResolvedRoot{Path: path, Origin: RootOriginPlatformDefault}, nil
}

// RootMarkerName is the marker's path inside the primary checkout's Git
// directory. It lives there rather than in the working tree because it is a fact
// about this machine's checkout and nothing a commit should carry.
const RootMarkerName = "yoyodyne/state-root"

// RootMarkerPath is where the state-root marker of a checkout is, and the Git
// directory it sits in. Both are empty for a directory that is not a Git
// checkout: there is nowhere to record a root, so nothing is recorded or
// compared. It reads the filesystem rather than asking Git, as configuration
// discovery does, so every process can answer it before anything else runs.
func RootMarkerPath(checkout string) (marker string, gitDirectory string, err error) {
	dotGit := filepath.Join(checkout, ".git")
	info, err := os.Stat(dotGit)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("inspect %s: %w", dotGit, err)
	}
	gitDirectory = dotGit
	if !info.IsDir() {
		// A linked worktree or a submodule: the file names the Git directory,
		// and a linked worktree's names the shared one in its commondir.
		content, err := os.ReadFile(dotGit)
		if err != nil {
			return "", "", fmt.Errorf("read %s: %w", dotGit, err)
		}
		line := strings.TrimSpace(string(content))
		named, ok := strings.CutPrefix(line, "gitdir:")
		if !ok {
			return "", "", fmt.Errorf("%s names no Git directory", dotGit)
		}
		gitDirectory = strings.TrimSpace(named)
		if !filepath.IsAbs(gitDirectory) {
			gitDirectory = filepath.Join(checkout, gitDirectory)
		}
		if common, err := os.ReadFile(filepath.Join(gitDirectory, "commondir")); err == nil {
			shared := strings.TrimSpace(string(common))
			if !filepath.IsAbs(shared) {
				shared = filepath.Join(gitDirectory, shared)
			}
			gitDirectory = shared
		}
		gitDirectory = filepath.Clean(gitDirectory)
	}
	return filepath.Join(gitDirectory, filepath.FromSlash(RootMarkerName)), gitDirectory, nil
}

// RootMarker is what one checkout's marker says: the root recorded, and where
// the record is. Recorded is empty where no process has recorded one yet, and
// Path is empty for a directory that is not a Git checkout.
type RootMarker struct {
	Path     string
	Recorded string
}

// ReadRootMarker reads a checkout's marker without writing it, for the surfaces
// that report whether it agrees rather than taking part in it.
func ReadRootMarker(checkout string) (RootMarker, error) {
	marker, _, err := RootMarkerPath(checkout)
	if err != nil || marker == "" {
		return RootMarker{}, err
	}
	content, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return RootMarker{Path: marker}, nil
	}
	if err != nil {
		return RootMarker{}, fmt.Errorf("read the state-root marker %s: %w", marker, err)
	}
	return RootMarker{Path: marker, Recorded: strings.TrimSpace(string(content))}, nil
}

// Agrees reports whether the marker leaves a process on this root free to
// start: nothing recorded yet, or the same directory recorded.
func (m RootMarker) Agrees(root string) bool {
	return m.Recorded == "" || sameRoot(m.Recorded, root)
}

// SplitRootError is the refusal the marker exists for: this process resolved a
// root other than the one the checkout's state already lives in.
type SplitRootError struct {
	Marker   string
	Recorded string
	Resolved ResolvedRoot
}

func (e *SplitRootError) Error() string {
	return fmt.Sprintf("this checkout's state is kept at %s, recorded in %s by an earlier yoyo process, "+
		"and this process resolved %s (from %s); one product's state is never split across two roots, so this refuses to start. "+
		"If %s is where it belongs, move it deliberately: stop the product with `yoyo stop`, move the directory, "+
		"change the setting, and remove %s. Otherwise remove whatever set the second root",
		e.Recorded, e.Marker, e.Resolved.Path, e.Resolved.Origin, e.Resolved.Path, e.Marker)
}

// AgreeRoot records the root this process resolved in the checkout's marker,
// or refuses with a SplitRootError where the marker already names a different
// one. It is called by every process that opens the state root for a product,
// before it opens anything under it. A checkout that is not a Git repository
// has nowhere to keep the marker and is let through.
//
// The marker is created rather than replaced, so of two processes starting at
// once on two roots exactly one records its root and the other reads it back
// and refuses.
func AgreeRoot(checkout string, resolved ResolvedRoot) error {
	marker, gitDirectory, err := RootMarkerPath(checkout)
	if err != nil || marker == "" {
		return err
	}
	root, err := repowrite.NewRoot(gitDirectory)
	if err != nil {
		return fmt.Errorf("record the state root in %s: %w", gitDirectory, err)
	}
	if _, _, err := root.CreateFile(RootMarkerName, []byte(resolved.Path+"\n")); err != nil {
		return fmt.Errorf("record the state root in %s: %w", marker, err)
	}
	read, err := ReadRootMarker(checkout)
	if err != nil {
		return err
	}
	if !read.Agrees(resolved.Path) {
		return &SplitRootError{Marker: read.Path, Recorded: read.Recorded, Resolved: resolved}
	}
	return nil
}

// sameRoot compares two roots as directories rather than as strings where both
// exist, so a root reached through a symlink is the root it links to.
func sameRoot(left, right string) bool {
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}
