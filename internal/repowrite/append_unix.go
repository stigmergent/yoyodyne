//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package repowrite

import (
	"os"
	"syscall"
)

// appendFlags open a file for appending and refuse a symlink standing where it
// goes. Directory traversal is confined by os.Root; O_NOFOLLOW additionally
// refuses a link planted at the resolved final component before the open.
const appendFlags = os.O_CREATE | os.O_WRONLY | os.O_APPEND | syscall.O_NOFOLLOW

// truncateFlags open an existing file to cut it, refusing a link at the final
// component for the same reason.
const truncateFlags = os.O_WRONLY | syscall.O_NOFOLLOW
