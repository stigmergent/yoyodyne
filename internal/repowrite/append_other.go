//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package repowrite

import "os"

// Refusing to follow a link at the final component is O_NOFOLLOW on the Unix
// hosts Yoyodyne supports. Elsewhere os.Root still confines the open, including
// a link planted after resolution. Platforms without that guarantee are refused
// by OpenPinnedRoot before any mutation.
const appendFlags = os.O_CREATE | os.O_WRONLY | os.O_APPEND

const truncateFlags = os.O_WRONLY
