//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd)

package beads

import "os"

func exportIdentity(os.FileInfo) (exportFileIdentity, bool) {
	return exportFileIdentity{}, false
}
