package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// The three readings the page polls — the standing, the throughput, and the
// spend — are built in the background, once per interval, and every request is
// served the latest one with the moment it was taken. Until yoyodyne-ifd.432.16
// each request built its reading from scratch: the whole tracker listing and
// every run record for every poll of every tab, which at a load average of 117
// took 6.4 seconds a standing and, under heavier load, past the tracker's own
// thirty-second timeout, so the page hung or errored exactly when the harness
// was busiest. Now two tabs cost one build, a slow build costs a request
// nothing, and a build that fails leaves the last reading served with the
// failure named beside its age rather than a refusal.

const (
	// standingInterval is how long after one standing build ends the next
	// starts. It is the page's own ten-second clock, so a tab polling at its
	// ordinary rate is handed a reading at most one interval and one build old.
	standingInterval = 10 * time.Second
	// minuteInterval is the throughput's and the spend's, which the page asks
	// for once a minute: the spend prices every event log a month holds.
	minuteInterval = time.Minute
	// idleAfter is how long a reading goes on being built with nobody asking
	// for it. A dashboard nobody has open builds nothing past it, and the next
	// request is served the last reading with its age and starts the building
	// again. It is longer than the page's slowest backed-off poll, so an open
	// tab never lets the building lapse.
	idleAfter = 5 * time.Minute
)

// SnapshotAge is what every reading served from a snapshot carries beside it,
// under "snapshot": when it was taken, how old it was when served, how often it
// is taken, whether it is older than two of those intervals, and — where the
// latest build since failed — what the failure said and when. The page shows
// the age from it and says so plainly when the reading is stale.
type SnapshotAge struct {
	TakenAt         time.Time `json:"taken_at"`
	AgeSeconds      int64     `json:"age_seconds"`
	IntervalSeconds int64     `json:"interval_seconds"`
	// Stale is the reading being older than two intervals: the builds are
	// falling behind or failing, and what is shown is not what the harness is
	// doing now.
	Stale bool `json:"stale"`
	// Failure is what the latest build said when it failed, after this reading
	// was taken. It is empty when the latest build succeeded.
	Failure  string     `json:"failure,omitempty"`
	FailedAt *time.Time `json:"failed_at,omitempty"`
}

// snapshot is one reading kept for every request. Its builder runs on its own
// goroutine, started by the first request that asks and stopping itself once
// nobody has asked for idleAfter, or when the server stops — so the load it
// carries is bounded by itself rather than by a cleanup somebody has to reach.
type snapshot struct {
	build    func(context.Context) (any, error)
	interval time.Duration
	idle     time.Duration
	now      func() time.Time
	// sleep waits the interval between builds, and says false where the
	// context ended first. It is a field so a test can hold the builder
	// between builds rather than waiting ten real seconds.
	sleep func(context.Context, time.Duration) bool

	mu sync.Mutex
	// life is what a build runs under: the server's lifetime once Serve has
	// started, and until then a context nothing cancels, which the idle bound
	// still stops.
	life context.Context
	// encoded is the latest reading, encoded once when it was built rather than
	// once per request; have is whether there is one at all.
	encoded  []byte
	have     bool
	takenAt  time.Time
	failure  string
	failedAt time.Time
	// attempted is closed once the first build has ended, either way. A
	// request before then waits for it, because there is nothing yet to serve.
	attempted chan struct{}
	running   bool
	askedAt   time.Time
}

func newSnapshot(interval time.Duration, build func(context.Context) (any, error)) *snapshot {
	return &snapshot{
		build:     build,
		interval:  interval,
		idle:      idleAfter,
		now:       time.Now,
		sleep:     sleep,
		life:      context.Background(),
		attempted: make(chan struct{}),
	}
}

// live sets the lifetime every later build runs under.
func (s *snapshot) live(ctx context.Context) {
	s.mu.Lock()
	s.life = ctx
	s.mu.Unlock()
}

// get is one request's reading: the latest one, encoded, with its age as of
// now. It never builds: it starts the builder where none is running, and waits
// only where no build has ever ended — the request's own context bounds that
// wait. Where the only builds there have been failed, the latest failure is the
// answer, as an error.
func (s *snapshot) get(ctx context.Context) ([]byte, SnapshotAge, error) {
	s.mu.Lock()
	s.askedAt = s.now()
	if !s.running {
		s.running = true
		go s.loop(s.life)
	}
	attempted := s.attempted
	s.mu.Unlock()

	select {
	case <-attempted:
	case <-ctx.Done():
		return nil, SnapshotAge{}, errors.New("the dashboard's first reading was still being taken when the request ended; the page asks again")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.have {
		return nil, SnapshotAge{}, errors.New(s.failure)
	}
	age := s.now().Sub(s.takenAt)
	if age < 0 {
		age = 0
	}
	served := SnapshotAge{
		TakenAt:         s.takenAt.UTC(),
		AgeSeconds:      int64(age / time.Second),
		IntervalSeconds: int64(s.interval / time.Second),
		Stale:           age > 2*s.interval,
	}
	if s.failure != "" {
		failedAt := s.failedAt.UTC()
		served.Failure = s.failure
		served.FailedAt = &failedAt
	}
	return s.encoded, served, nil
}

// loop builds, waits an interval from the end of the build, and builds again,
// until nobody has asked for idle or its context ends. Waiting from the end
// rather than the start is what keeps a build slower than its interval from
// running back to back and becoming the load it was taken to relieve.
func (s *snapshot) loop(ctx context.Context) {
	for {
		s.take(ctx)
		if !s.sleep(ctx, s.interval) {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		if s.now().Sub(s.askedAt) > s.idle {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
	}
}

// sleep is the real wait between builds.
func sleep(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// take is one build, recorded. A success replaces the reading and clears any
// failure; a failure keeps the reading and records what it said.
func (s *snapshot) take(ctx context.Context) {
	started := s.now()
	reading, err := s.build(ctx)
	var encoded []byte
	if err == nil {
		encoded, err = encode(reading)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.failure = err.Error()
		s.failedAt = s.now()
	} else {
		s.encoded = encoded
		s.have = true
		s.takenAt = started
		s.failure = ""
		s.failedAt = time.Time{}
	}
	select {
	case <-s.attempted:
	default:
		close(s.attempted)
	}
}

// encode is a reading as the JSON object it is served as, with tags escaped as
// every answer's are.
func encode(reading any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(reading); err != nil {
		return nil, err
	}
	encoded := bytes.TrimSpace(buffer.Bytes())
	if len(encoded) < 2 || encoded[0] != '{' {
		return nil, errors.New("the reading did not encode as a JSON object")
	}
	return encoded, nil
}

// withAge is the encoded reading with its age added as a "snapshot" member,
// first, so the reading's own fields are exactly what they were and a tool
// that read them before reads them unchanged.
func withAge(encoded []byte, age SnapshotAge) ([]byte, error) {
	meta, err := json.Marshal(age)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(len(encoded) + len(meta) + 16)
	out.WriteString(`{"snapshot":`)
	out.Write(meta)
	rest := bytes.TrimSpace(encoded[1:])
	if len(rest) > 0 && rest[0] != '}' {
		out.WriteByte(',')
	}
	out.Write(rest)
	out.WriteByte('\n')
	return out.Bytes(), nil
}
