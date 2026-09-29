package runstate

// Which keys each running part of the product can read of the configuration.
//
// A part of the product runs the build it was started from for days, and it
// reads the configuration again and again while it does. A landing that adds a
// key to the configuration therefore reaches the file at once and the parts
// only when each is restarted, and a part whose build predates the key refuses
// the whole file: on 2026-09-28 the dashboard served nothing but "field effort
// not found in type config.agentDocument" for two hours, and nothing said
// which part was behind or why.
//
// So each part records, as it starts, which build it is, which file it reads,
// and the configuration keys its build can read. Any later build — `yoyo
// doctor`, `yoyo config validate`, the read model — can then compare the file
// as it stands against every part still running, without the older build's
// types, and name the part, its build, and the keys. A record whose process
// has gone is a part that is not running and says nothing.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/domain"
)

// ConfigReaderSchemaVersion is 1 and has never changed.
const ConfigReaderSchemaVersion = 1

// ConfigReaderSupervisor is the supervisor's name among the parts that record
// what they read. The other parts are named as the services section names
// them.
const ConfigReaderSupervisor = "supervisor"

const configReadersDirectory = "config-readers"

// ConfigReader is one running part's account of what its build reads.
type ConfigReader struct {
	SchemaVersion int              `json:"schema_version"`
	ProductID     domain.ProductID `json:"product_id"`
	// Service is the part: a service the configuration's services section
	// names, or the supervisor.
	Service string `json:"service"`
	PID     int    `json:"pid"`
	// Build is the revision the part was built from, and empty where the build
	// recorded none.
	Build string `json:"build,omitempty"`
	// ConfigPath is the configuration file the part reads, as it resolved it.
	ConfigPath string    `json:"config_path"`
	StartedAt  time.Time `json:"started_at"`
	// Keys is config.SchemaKeys as the part's own build derived it.
	Keys []string `json:"keys"`
}

func (r ConfigReader) Validate() error {
	var problems []error
	if r.SchemaVersion != ConfigReaderSchemaVersion {
		problems = append(problems, fmt.Errorf("configuration reader schema version %d is not supported", r.SchemaVersion))
	}
	if err := domain.ValidateIdentifier("product id", string(r.ProductID)); err != nil {
		problems = append(problems, err)
	}
	if !knownConfigReader(r.Service) {
		problems = append(problems, fmt.Errorf("configuration reader %q is not a part of the product", r.Service))
	}
	if r.PID <= 0 {
		problems = append(problems, fmt.Errorf("configuration reader pid %d names no process", r.PID))
	}
	if !filepath.IsAbs(r.ConfigPath) {
		problems = append(problems, fmt.Errorf("configuration reader's file %q is not an absolute path", r.ConfigPath))
	}
	if r.StartedAt.IsZero() {
		problems = append(problems, errors.New("started at is required"))
	}
	if len(r.Keys) == 0 {
		problems = append(problems, errors.New("a configuration reader names no keys"))
	}
	return errors.Join(problems...)
}

func knownConfigReader(service string) bool {
	if service == ConfigReaderSupervisor {
		return true
	}
	for _, name := range config.ServiceNames {
		if service == string(name) {
			return true
		}
	}
	return false
}

// ConfigMismatch is a running part whose build cannot read keys the file it
// reads now carries.
type ConfigMismatch struct {
	Service    string    `json:"service"`
	PID        int       `json:"pid"`
	Build      string    `json:"build,omitempty"`
	ConfigPath string    `json:"config_path"`
	StartedAt  time.Time `json:"started_at"`
	// Keys are the file's keys the part's build does not read, with the file's
	// own names in them.
	Keys []string `json:"keys"`
}

// Says is the mismatch in a sentence: the part, its build, and the keys.
func (m ConfigMismatch) Says() string {
	build := "a build that recorded no revision"
	if m.Build != "" {
		build = "build " + shortBuild(m.Build)
	}
	return fmt.Sprintf("the %s service, running %s as pid %d since %s, cannot read %s in %s, so every read it makes of the configuration fails",
		m.Service, build, m.PID, m.StartedAt.Local().Format("2006-01-02 15:04 MST"), strings.Join(m.Keys, ", "), m.ConfigPath)
}

// ConfigMismatchRemedy says what brings a part onto a build that reads the
// file, in the words every surface uses for it.
func ConfigMismatchRemedy(service string) string {
	switch service {
	case string(config.ServiceScheduler):
		return "the watch restarts itself into the deployed build between runs, once the binary on disk is one that reads every key; `yoyo stop` and `yoyo start` restart it now"
	case string(config.ServiceSlack):
		return "the supervisor restarts the sink into the deployed build between its passes, once the binary on disk is one that reads every key; `yoyo stop` and `yoyo start` restart it now"
	case string(config.ServiceDashboard):
		return "nothing restarts the dashboard onto a newer build until the supervisor adopts it (the dashboard's adoption, yoyodyne-ifd.414), so stop it and start it again with `yoyo dashboard`"
	case ConfigReaderSupervisor:
		return "nothing restarts the supervisor onto a newer build, so `yoyo stop` and `yoyo start` start it again from the binary on disk"
	}
	return "restart it from the binary on disk"
}

func shortBuild(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

// ConfigReaderStore is where the parts' records live: one file per part under
// the product.
//
// It writes with the temporary-file-and-rename every state file beside it
// uses, directly rather than through the repository's confined-write
// primitive, for the reason the supervision store gives: the state root is the
// harness's own directory outside every repository.
type ConfigReaderStore struct {
	root      string
	productID domain.ProductID
	// running answers whether a recorded process is still there; nil asks the
	// operating system.
	running func(pid int) (bool, error)
}

func NewConfigReaderStore(root string, productID domain.ProductID) (*ConfigReaderStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("state root must be an absolute path")
	}
	if err := domain.ValidateIdentifier("product id", string(productID)); err != nil {
		return nil, err
	}
	return &ConfigReaderStore{
		root:      filepath.Join(filepath.Clean(root), "products", string(productID), configReadersDirectory),
		productID: productID,
	}, nil
}

// WithProcessCheck replaces the question of whether a recorded process is
// still running, for a test that records a part no process is behind.
func (s *ConfigReaderStore) WithProcessCheck(running func(pid int) (bool, error)) *ConfigReaderStore {
	copied := *s
	copied.running = running
	return &copied
}

// Record writes this part's account, replacing whatever the part's previous
// process wrote.
func (s *ConfigReaderStore) Record(reader ConfigReader) error {
	reader.SchemaVersion = ConfigReaderSchemaVersion
	reader.ProductID = s.productID
	if err := reader.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create configuration reader directory: %w", err)
	}
	temporary, err := os.CreateTemp(s.root, ".reader-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration reader record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary configuration reader record: %w", err)
	}
	if err := writeJSONFile(temporary, "configuration reader record", reader); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration reader record: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(s.root, reader.Service+".json")); err != nil {
		return fmt.Errorf("replace configuration reader record: %w", err)
	}
	return syncDirectory(s.root)
}

// Running is every part whose recorded process is still there, in the order
// of their names. A record a newer build wrote is read tolerantly, for the
// reason the supervision record is: the reader is often the older build.
func (s *ConfigReaderStore) Running() ([]ConfigReader, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read configuration reader records: %w", err)
	}
	running := s.running
	if running == nil {
		running = processIsRunning
	}
	var readers []ConfigReader
	var problems []error
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		reader, err := s.load(filepath.Join(s.root, name))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		alive, err := running(reader.PID)
		if err != nil {
			problems = append(problems, fmt.Errorf("whether the %s service's process %d is running could not be read: %w", reader.Service, reader.PID, err))
			continue
		}
		if alive {
			readers = append(readers, reader)
		}
	}
	sort.Slice(readers, func(i, j int) bool { return readers[i].Service < readers[j].Service })
	return readers, errors.Join(problems...)
}

func (s *ConfigReaderStore) load(path string) (ConfigReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return ConfigReader{}, fmt.Errorf("open configuration reader record: %w", err)
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, maxEncodedStateBytes))
	if err != nil {
		return ConfigReader{}, fmt.Errorf("read configuration reader record %s: %w", path, err)
	}
	var reader ConfigReader
	unknown, err := decodeTolerating(encoded, &reader)
	if err != nil {
		return ConfigReader{}, fmt.Errorf("decode configuration reader record %s: %w", path, err)
	}
	noteUnknownFields("configuration reader record", unknown)
	if err := reader.Validate(); err != nil {
		return ConfigReader{}, fmt.Errorf("configuration reader record %s: %w", path, err)
	}
	if reader.ProductID != s.productID {
		return ConfigReader{}, fmt.Errorf("configuration reader record %s belongs to product %q, not %q", path, reader.ProductID, s.productID)
	}
	return reader, nil
}

// Mismatches compares the file each running part reads, as it stands now,
// against the keys that part's build reads, and returns every part that
// cannot read something in it. A part whose file cannot be read is a problem
// in the returned error rather than a part reported current.
func (s *ConfigReaderStore) Mismatches() ([]ConfigMismatch, error) {
	return s.MismatchesIn(os.ReadFile)
}

// MismatchesIn is Mismatches with the file each part reads read by read
// rather than from the working tree: a landing reads it as the commit it
// landed holds it, because the checkout the parts read may not have moved onto
// that commit yet.
func (s *ConfigReaderStore) MismatchesIn(read func(configPath string) ([]byte, error)) ([]ConfigMismatch, error) {
	readers, err := s.Running()
	problems := []error{err}
	var mismatches []ConfigMismatch
	for _, reader := range readers {
		source, readErr := read(reader.ConfigPath)
		if readErr != nil {
			problems = append(problems, fmt.Errorf("the configuration the %s service reads could not be read: %w", reader.Service, readErr))
			continue
		}
		keys, keysErr := config.UnreadableKeys(source, reader.Keys)
		if keysErr != nil {
			problems = append(problems, fmt.Errorf("the configuration the %s service reads: %w", reader.Service, keysErr))
			continue
		}
		if len(keys) == 0 {
			continue
		}
		mismatches = append(mismatches, ConfigMismatch{
			Service:    reader.Service,
			PID:        reader.PID,
			Build:      reader.Build,
			ConfigPath: reader.ConfigPath,
			StartedAt:  reader.StartedAt,
			Keys:       keys,
		})
	}
	return mismatches, errors.Join(problems...)
}
