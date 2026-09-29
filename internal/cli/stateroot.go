package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func runStateRoot(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "rebind" {
		printStateRootUsage(stderr)
		return 2
	}
	return rebindStateRoot(args[1:], stdout, stderr)
}

// rebindStateRoot replaces a checkout's state-root marker that names a root
// gone from disk with the root this shell resolves. It is the one place besides
// productStateRoot that writes the marker, and it goes no further than the
// marker: a marker naming a root still on disk is refused exactly as every
// other command refuses it, because moving a product off state that is there is
// a deliberate move rather than a repair.
func rebindStateRoot(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("state-root rebind", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "", "configuration file path (default: the nearest project configuration)")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	positional, err := parseArguments(flags, args)
	if err != nil {
		return 2
	}
	if len(positional) != 0 {
		fmt.Fprintln(stderr, "state-root rebind does not accept positional arguments")
		return 2
	}
	fail := func(err error) int {
		if *jsonOutput {
			if code := writeJSON(stdout, stderr, map[string]string{"error": err.Error()}); code != 0 {
				return code
			}
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	resolved, err := loadConfiguration(*path)
	if err != nil {
		return fail(err)
	}
	repository, err := resolvePath(config.ProjectDirectory(resolved.Path), resolved.Config.Product.Repository)
	if err != nil {
		return fail(fmt.Errorf("resolve product repository: %w", err))
	}
	root, err := runstate.ResolveRoot(os.Getenv, os.UserHomeDir, runtime.GOOS)
	if err != nil {
		return fail(err)
	}
	rebinding, err := runstate.RebindRoot(repository, root)
	if err != nil {
		return fail(err)
	}
	said := rebindingSays(rebinding)
	if *jsonOutput {
		return writeJSON(stdout, stderr, map[string]any{
			"marker":   rebinding.Before.Path,
			"before":   rebinding.Before.Recorded,
			"root":     rebinding.Root.Path,
			"origin":   rebinding.Root.Origin,
			"replaced": rebinding.Replaced,
			"says":     said,
		})
	}
	fmt.Fprintln(stdout, said)
	return 0
}

func rebindingSays(rebinding runstate.Rebinding) string {
	switch {
	case rebinding.Replaced:
		return fmt.Sprintf("rebound %s to %s, from %s: it named %s, which no longer exists, recorded %s",
			rebinding.Before.Path, rebinding.Root.Path, rebinding.Root.Origin, rebinding.Before.Recorded, rebinding.Before.WrittenBy())
	case rebinding.Before.Recorded == "":
		return fmt.Sprintf("%s recorded no root, and now records %s, from %s", rebinding.Before.Path, rebinding.Root.Path, rebinding.Root.Origin)
	default:
		return fmt.Sprintf("%s already names %s, from %s; nothing to rebind", rebinding.Before.Path, rebinding.Root.Path, rebinding.Root.Origin)
	}
}

func printStateRootUsage(writer io.Writer) {
	fmt.Fprintln(writer, strings.TrimSpace(`
Usage: yoyo state-root rebind [options]

Replace this checkout's state-root marker, .git/yoyodyne/state-root, with the
state root this shell resolves, where the marker names a root that no longer
exists. Such a marker makes every command from the checkout refuse to start,
and splits nothing, because there is no state at the root it names. A marker
naming a root that is still on disk is refused rather than replaced: move the
state deliberately first (yoyo stop, move the directory, change the setting),
and then rebind. A marker that already agrees, or a checkout with none, is left
as it is.

Options:
  --config <path>   configuration file (default: the nearest .yoyodyne/config.yaml)
  --json            emit machine-readable JSON`))
}
