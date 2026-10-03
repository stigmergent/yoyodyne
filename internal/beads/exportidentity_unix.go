//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package beads

import (
	"os"
	"syscall"
)

func exportIdentity(info os.FileInfo) (exportFileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return exportFileIdentity{}, false
	}
	return exportFileIdentity{uint64(stat.Dev), uint64(stat.Ino)}, true
}
