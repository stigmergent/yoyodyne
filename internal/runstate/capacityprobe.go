package runstate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

// CapacityProbeStore reserves paced probes before they spend. The reservation
// survives a crash or supervisor restart, and concurrent schedulers share it.
type CapacityProbeStore struct {
	root      string
	productID domain.ProductID
}

type capacityProbeRecord struct {
	SchemaVersion int              `json:"schema_version"`
	ProductID     domain.ProductID `json:"product_id"`
	Scope         string           `json:"scope"`
	Next          time.Time        `json:"next"`
}

func NewCapacityProbeStore(root string, productID domain.ProductID) (*CapacityProbeStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("state root must be an absolute path")
	}
	if err := domain.ValidateIdentifier("product id", string(productID)); err != nil {
		return nil, err
	}
	return &CapacityProbeStore{root: root, productID: productID}, nil
}

func (s *CapacityProbeStore) directory() string {
	return filepath.Join("products", string(s.productID))
}
func (s *CapacityProbeStore) relativePath() string {
	return filepath.Join(s.directory(), "capacity-probes.jsonl")
}
func (s *CapacityProbeStore) Path() string { return filepath.Join(s.root, s.relativePath()) }

func (s *CapacityProbeStore) Next() (map[string]time.Time, error) {
	confined, err := repowrite.OpenPinnedRoot(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]time.Time{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer confined.Close()
	next, _, err := s.next(confined)
	return next, err
}

// next ignores only an incomplete last append. Claim removes that tail under
// the lock before recording a new reservation. A complete invalid record fails
// the gate rather than silently losing pacing information.
func (s *CapacityProbeStore) next(confined *repowrite.PinnedRoot) (map[string]time.Time, int64, error) {
	encoded, err := confined.ReadFile(s.relativePath())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]time.Time{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	complete := bytes.LastIndexByte(encoded, '\n') + 1
	next := map[string]time.Time{}
	for _, line := range bytes.Split(encoded[:complete], []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var record capacityProbeRecord
		if err := decodeStrictly(line, &record); err != nil {
			return nil, 0, err
		}
		if record.SchemaVersion != 1 || record.ProductID != s.productID {
			return nil, 0, errors.New("capacity probe record has the wrong schema or product")
		}
		if record.Scope == "" || record.Next.IsZero() {
			return nil, 0, errors.New("capacity probe reservation names no scope or time")
		}
		if record.Next.After(next[record.Scope]) {
			next[record.Scope] = record.Next
		}
	}
	return next, int64(complete), nil
}

// Claim moves the next probe time atomically before the invocation. Failed and
// interrupted probes wait the same interval as refusals; they never open a window.
func (s *CapacityProbeStore) Claim(ctx context.Context, key string, now time.Time, interval time.Duration) (time.Time, bool, error) {
	if key == "" || now.IsZero() || interval <= 0 {
		return time.Time{}, false, errors.New("capacity probe requires a scope, time, and positive interval")
	}
	confined, err := repowrite.OpenPinnedRoot(s.root)
	if err != nil {
		return time.Time{}, false, err
	}
	defer confined.Close()
	if err := confined.MakeDirectory(s.directory(), 0o700); err != nil {
		return time.Time{}, false, err
	}
	file, err := confined.OpenLock(s.relativePath() + ".lock")
	if err != nil {
		return time.Time{}, false, err
	}
	lockCtx, cancel := context.WithTimeout(ctx, intakeLockWait)
	defer cancel()
	if err := lockStateFile(lockCtx, file); err != nil {
		file.Close()
		return time.Time{}, false, err
	}
	defer releaseStateFile(file)
	next, complete, err := s.next(confined)
	if err != nil {
		return time.Time{}, false, err
	}
	if now.Before(next[key]) {
		return next[key], false, nil
	}
	deadline := now.Add(interval).UTC()
	encoded, err := json.Marshal(capacityProbeRecord{1, s.productID, key, deadline})
	if err != nil {
		return time.Time{}, false, err
	}
	if err := confined.AppendRecord(s.relativePath(), append(encoded, '\n'), complete); err != nil {
		return time.Time{}, false, fmt.Errorf("preserve capacity probe reservation: %w", err)
	}
	return deadline, true, nil
}
