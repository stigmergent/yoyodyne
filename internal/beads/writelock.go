package beads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

// writeLockDirectory is where the lock each item's writes queue on lives,
// relative to the directory bd is run in. It is under the store's own directory
// because that is what every writer to one store shares whatever process it is,
// and bd's own ignore file there already keeps `*.lock` out of the repository.
const writeLockDirectory = ".beads/yoyodyne-writes"

// write runs one bd invocation that changes the item id names, holding that
// item's write lock for as long as bd runs.
//
// bd does not serialize two writes to one item. An append is a read of the notes
// already there and a write of them with the new line added, and a metadata key
// is set the same way, so two that overlap each read the same notes and the one
// that finishes second writes back over the first. Both exit 0, and each one's
// own answer carries its own line, so nothing a writer can read off its own
// invocation says the other was lost: against bd 1.1.2, six appends and three
// other writes made at once to one item lost an append or a metadata key in 4
// of 15 batches, every invocation reporting success
// (docs/diagnoses/yoyodyne-ifd-433-23-concurrent-writes-to-one-item.md). A
// lost append is a lost outcome, price, or goal attribution, which is the loss
// nobody finds.
//
// So writes to one item queue here, across processes: the lock is an advisory
// lock on a file every client of the same store opens, which the operating
// system drops when its holder exits, so a writer that dies holds nothing.
// Writes to different items do not queue on each other — nothing was lost
// between them — so a bd invocation that stalls holds up only the writes to its
// own item. The wait is bounded by the client's own bound on an invocation,
// and a write that waited it out fails rather than running unqueued.
//
// Where the directory bd runs in holds no store there is nothing to share a lock
// with, and bd is left to say so itself.
func (c Client) write(ctx context.Context, id string, args ...string) ([]byte, error) {
	unlock, err := c.lockItem(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return c.run(ctx, args...)
}

// lockItem takes the write lock for one item, waiting for whoever holds it, and
// returns what releases it.
func (c Client) lockItem(ctx context.Context, id string) (func(), error) {
	if err := validateIssueID(id); err != nil {
		return nil, err
	}
	if info, err := os.Stat(filepath.Join(c.Dir, ".beads")); err != nil || !info.IsDir() {
		return func() {}, nil
	}
	dir := c.Dir
	if dir == "" {
		dir = "."
	}
	root, err := repowrite.NewRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("lock writes to %s: %w", id, err)
	}
	file, err := root.OpenAppend(writeLockDirectory+"/"+id+".lock", 0o600, 0o700)
	if err != nil {
		return nil, fmt.Errorf("lock writes to %s: %w", id, err)
	}
	waiting, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	if err := lockWriteFile(waiting, file); err != nil {
		_ = file.Close()
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, fmt.Errorf("another write to %s held it for longer than %s, so this one was not made: %w", id, c.timeout(), err)
		}
		return nil, fmt.Errorf("lock writes to %s: %w", id, err)
	}
	return func() {
		_ = unlockWriteFile(file)
		_ = file.Close()
	}, nil
}
