package runstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
)

// CapacityProbeStore reserves paced probes before they spend. The reservation
// survives a crash or supervisor restart, and concurrent schedulers share it.
type CapacityProbeStore struct {
	root      string
	productID domain.ProductID
}

type capacityProbeRecord struct {
	SchemaVersion int                  `json:"schema_version"`
	ProductID     domain.ProductID     `json:"product_id"`
	Next          map[string]time.Time `json:"next"`
}

func NewCapacityProbeStore(root string, productID domain.ProductID) (*CapacityProbeStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("state root must be an absolute path")
	}
	if err := domain.ValidateIdentifier("product id", string(productID)); err != nil {
		return nil, err
	}
	return &CapacityProbeStore{root: filepath.Join(root, "products", string(productID)), productID: productID}, nil
}

func (s *CapacityProbeStore) Path() string { return filepath.Join(s.root, "capacity-probes.json") }

func (s *CapacityProbeStore) Next() (map[string]time.Time, error) {
	encoded, err := os.ReadFile(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]time.Time{}, nil
	}
	if err != nil {
		return nil, err
	}
	var record capacityProbeRecord
	if _, err := decodeStrict(encoded, &record); err != nil {
		return nil, err
	}
	if record.SchemaVersion != 1 || record.ProductID != s.productID {
		return nil, errors.New("capacity probe record has the wrong schema or product")
	}
	for key, next := range record.Next {
		if key == "" || next.IsZero() {
			return nil, errors.New("capacity probe reservation names no scope or time")
		}
	}
	return record.Next, nil
}

// Claim moves the next probe time atomically before the invocation. Failed and
// interrupted probes wait the same interval as refusals; they never open a window.
func (s *CapacityProbeStore) Claim(ctx context.Context, key string, now time.Time, interval time.Duration) (time.Time, bool, error) {
	if key == "" || now.IsZero() || interval <= 0 {
		return time.Time{}, false, errors.New("capacity probe requires a scope, time, and positive interval")
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return time.Time{}, false, err
	}
	file, err := os.OpenFile(s.Path()+".lock", os.O_CREATE|os.O_RDWR, 0o600)
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
	next, err := s.Next()
	if err != nil {
		return time.Time{}, false, err
	}
	if now.Before(next[key]) {
		return next[key], false, nil
	}
	if next == nil {
		next = map[string]time.Time{}
	}
	deadline := now.Add(interval).UTC()
	next[key] = deadline
	temporary, err := os.CreateTemp(s.root, ".capacity-probes-*.tmp")
	if err != nil {
		return time.Time{}, false, err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return time.Time{}, false, err
	}
	if err := writeJSONFile(temporary, "capacity probe record", capacityProbeRecord{1, s.productID, next}); err != nil {
		temporary.Close()
		return time.Time{}, false, err
	}
	if err := temporary.Close(); err != nil {
		return time.Time{}, false, err
	}
	if err := os.Rename(temporary.Name(), s.Path()); err != nil {
		return time.Time{}, false, err
	}
	if err := syncDirectory(s.root); err != nil {
		return time.Time{}, false, fmt.Errorf("sync capacity probe reservation: %w", err)
	}
	return deadline, true, nil
}
