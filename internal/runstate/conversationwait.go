package runstate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

// ConversationHeldError names the turn a claim waited behind. Cause is the
// cancellation or deadline that ended the claim, and nil for a refusing claim.
type ConversationHeldError struct {
	Identity ConversationIdentity
	PID      int
	HeldAt   time.Time
	Cause    error
}

func (e *ConversationHeldError) Error() string {
	said := fmt.Sprintf("the %s conversation is %s", e.Identity, ErrConversationHeld)
	if e.PID > 0 {
		said += fmt.Sprintf(" (holder: process %d, since %s)", e.PID, e.HeldAt.Local().Format("2006-01-02 15:04:05 MST"))
	} else {
		said += " (the holder could not be read)"
	}
	if e.Cause != nil {
		said += ": waiting for its turn ended: " + e.Cause.Error()
	}
	return said
}

func (e *ConversationHeldError) Unwrap() error {
	return errors.Join(ErrConversationHeld, e.Cause)
}

// ConversationWait is a live turn waiting for a provider, with its conversation
// available to other turns. Each wait has its own record so several waiting
// turns cannot replace one another or the conversation's completed turns.
type ConversationWait struct {
	Agent          string           `json:"agent"`
	Role           domain.AgentRole `json:"role"`
	ConversationID string           `json:"conversation_id"`
	PID            int              `json:"pid"`
	Since          time.Time        `json:"since"`
	Reason         string           `json:"reason"`
}

// Waiting records what this hold's turn waits on. The caller clears the record
// after taking the conversation back, or after abandoning the wait. A crashed
// process's record is ignored by WaitingTurns, as a crashed holder's stamp is.
func (h *ConversationHold) Waiting(conversationID, reason string, since time.Time) (func() error, error) {
	if h == nil || !h.Held() {
		return nil, errors.New("recording a conversation wait requires its hold")
	}
	if !conversationIDPattern.MatchString(conversationID) || strings.TrimSpace(reason) == "" || len(reason) > MaxSweepTextBytes || since.IsZero() {
		return nil, errors.New("a conversation wait names its conversation, reason, and time")
	}
	id, err := NewConversationID()
	if err != nil {
		return nil, err
	}
	name := ".waiting-" + id + ".json"
	wait := ConversationWait{Agent: h.identity.Agent, Role: h.identity.Role, ConversationID: conversationID,
		PID: os.Getpid(), Since: since.UTC(), Reason: reason}
	if wait.Agent == "" {
		wait.Agent = string(wait.Role)
	}
	root, err := h.store.pinWaitRoot()
	if err != nil {
		return nil, fmt.Errorf("pin the conversation wait directory: %w", err)
	}
	if err := h.store.recordWaitIn(root, name, wait); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	// Keep the directory pinned until cleanup, including while the turn has
	// released its conversation. A replaced parent must never redirect removal.
	var once sync.Once
	var cleared error
	return func() error {
		once.Do(func() {
			removeErr := root.Remove(name)
			if errors.Is(removeErr, os.ErrNotExist) {
				removeErr = nil
			}
			cleared = errors.Join(removeErr, root.Sync(), root.Close())
			if cleared != nil {
				cleared = fmt.Errorf("clear the conversation wait: %w", cleared)
			}
		})
		return cleared
	}, nil
}

func (s *ConversationStore) pinWaitRoot() (*repowrite.PinnedRoot, error) {
	return pinStateRoot(s.root, s.anchor)
}

func (s *ConversationStore) recordWaitIn(root *repowrite.PinnedRoot, name string, wait ConversationWait) error {
	encoded, err := encodeRecord("conversation wait", wait)
	if err != nil {
		return err
	}
	if err := root.Unchanged(); err != nil {
		return err
	}
	if err := root.CreateFile(name, encoded, 0o600); err != nil {
		return fmt.Errorf("record the conversation wait: %w", err)
	}
	if err := root.Unchanged(); err != nil {
		return errors.Join(err, root.Remove(name), root.Sync())
	}
	return nil
}

// WaitingTurns observes the waits without claiming any conversation. The record
// is written before releasing the hold, so it also names a waiting turn whose
// release has not yet happened, or which is taking the hold back to resume.
func (s *ConversationStore) WaitingTurns() ([]ConversationWait, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var waits []ConversationWait
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".waiting-chat-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		file, err := os.Open(filepath.Join(s.root, entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue // The turn resumed during this reading.
		}
		if err != nil {
			return waits, err
		}
		encoded, readErr := io.ReadAll(io.LimitReader(file, maxEncodedStateBytes+1))
		file.Close()
		if readErr != nil {
			return waits, fmt.Errorf("read conversation wait %s: %w", entry.Name(), readErr)
		}
		if len(encoded) > maxEncodedStateBytes {
			return waits, fmt.Errorf("conversation wait %s exceeds %d bytes", entry.Name(), maxEncodedStateBytes)
		}
		var wait ConversationWait
		unknown, decodeErr := decodeTolerating(encoded, &wait)
		if decodeErr != nil {
			return waits, fmt.Errorf("read conversation wait %s: %w", entry.Name(), decodeErr)
		}
		noteUnknownFields("conversation wait", unknown)
		identity := ConversationIdentity{Agent: wait.Agent, Role: wait.Role}
		if wait.PID <= 0 || !conversationIDPattern.MatchString(wait.ConversationID) || wait.Since.IsZero() || strings.TrimSpace(wait.Reason) == "" || len(wait.Reason) > MaxSweepTextBytes || identity.validate() != nil {
			return waits, fmt.Errorf("invalid conversation wait %s", entry.Name())
		}
		running, err := processIsRunning(wait.PID)
		if err != nil {
			return waits, err
		}
		if running {
			waits = append(waits, wait)
		}
	}
	sort.Slice(waits, func(i, j int) bool {
		if waits[i].Agent != waits[j].Agent {
			return waits[i].Agent < waits[j].Agent
		}
		return waits[i].Since.Before(waits[j].Since)
	})
	return waits, nil
}
