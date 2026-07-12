package immichimport

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"gpix/pkg/store"

	_ "modernc.org/sqlite"
)

// Progress is a snapshot of an import job for the UI.
type Progress struct {
	Running    bool      `json:"running"`
	Server     string    `json:"server"`
	Workers    int       `json:"workers"`
	Total      int       `json:"total"`
	Done       int       `json:"done"`
	Skipped    int       `json:"skipped"`
	Failed     int       `json:"failed"`
	Current    string    `json:"current"`
	Error      string    `json:"error"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

const maxWorkers = 10

// Importer copies assets from an Immich server into gpix via the object store
// (so encryption/disguise/dedup all apply). One job runs at a time. Already-
// imported asset IDs are remembered so re-runs are cheap and idempotent.
type Importer struct {
	be  store.Backend
	db  *sql.DB
	log *slog.Logger

	mu     sync.Mutex
	prog   Progress
	cancel context.CancelFunc
}

// New opens the import-state database and returns an Importer.
func New(be store.Backend, dbPath string, log *slog.Logger) (*Importer, error) {
	if log == nil {
		log = slog.Default()
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS imported (
    server     TEXT NOT NULL,
    asset_id   TEXT NOT NULL,
    imported_at INTEGER NOT NULL,
    PRIMARY KEY (server, asset_id)
);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Importer{be: be, db: db, log: log}, nil
}

func (im *Importer) Close() error { return im.db.Close() }

// Status returns the current job snapshot.
func (im *Importer) Status() Progress {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.prog
}

// Stop cancels a running import (best effort).
func (im *Importer) Stop() {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.cancel != nil {
		im.cancel()
	}
}

// Start kicks off an import from an Immich server, in the background, using
// `workers` parallel transfers (clamped to 1..maxWorkers).
func (im *Importer) Start(serverURL, apiKey string, workers int) error {
	return im.start(serverURL, immichSource{c: NewClient(serverURL, apiKey)}, workers)
}

// StartGoogle imports the whole library of another Google account, given its
// GP_AUTH_DATA, into the current gpix (encryption/disguise/dedup apply).
func (im *Importer) StartGoogle(authData string, workers int) error {
	src, err := newGoogleSource(authData)
	if err != nil {
		return err
	}
	return im.start("Google account", src, workers)
}

func (im *Importer) start(display string, src source, workers int) error {
	if workers < 1 {
		workers = 1
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}
	im.mu.Lock()
	if im.prog.Running {
		im.mu.Unlock()
		return errors.New("an import is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	im.cancel = cancel
	im.prog = Progress{Running: true, Server: display, Workers: workers, StartedAt: time.Now()}
	im.mu.Unlock()

	go im.run(ctx, src, workers)
	return nil
}

func (im *Importer) run(ctx context.Context, src source, workers int) {
	defer func() {
		im.mu.Lock()
		im.prog.Running = false
		im.prog.FinishedAt = time.Now()
		im.cancel = nil
		im.mu.Unlock()
	}()

	if err := src.ping(ctx); err != nil {
		im.setError("connect: " + err.Error())
		return
	}
	label := src.label()

	// Worker pool: the producer enumerates the source and feeds items to
	// `workers` goroutines that download + upload in parallel.
	ch := make(chan srcItem, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				if ctx.Err() != nil {
					return
				}
				im.processItem(ctx, src, label, it)
			}
		}()
	}

	queued := 0
	err := src.enumerate(ctx,
		func(total int) {
			im.mu.Lock()
			im.prog.Total = total
			im.mu.Unlock()
		},
		func(it srcItem) bool {
			select {
			case ch <- it:
				queued++
				// Periodically return transient buffers to the OS so a long,
				// highly parallel import doesn't let RSS balloon (Go keeps a large
				// freed heap by default; also honors GOMEMLIMIT).
				if queued%200 == 0 {
					debug.FreeOSMemory()
				}
				return true
			case <-ctx.Done():
				return false
			}
		})
	if err != nil {
		im.setError(err.Error())
	}
	close(ch)
	wg.Wait()
	debug.FreeOSMemory()
}

func (im *Importer) processItem(ctx context.Context, src source, label string, it srcItem) {
	im.mu.Lock()
	im.prog.Current = it.name
	im.mu.Unlock()

	if im.alreadyImported(label, it.id) {
		im.count(func(p *Progress) { p.Skipped++ })
		return
	}

	rc, fname, err := src.download(ctx, it.id)
	if err != nil {
		im.fail(err)
		return
	}
	defer rc.Close()

	name := it.name
	if name == "" {
		name = fname // some sources report the filename only at download time
	}
	if name == "" {
		name = it.id
	}
	if _, err := im.be.Put(ctx, name, rc, -1, ""); err != nil {
		im.fail(err)
		return
	}
	im.markImported(label, it.id)
	im.count(func(p *Progress) { p.Done++ })
}

func (im *Importer) alreadyImported(server, id string) bool {
	var one int
	err := im.db.QueryRow(`SELECT 1 FROM imported WHERE server = ? AND asset_id = ?`, server, id).Scan(&one)
	return err == nil
}

func (im *Importer) markImported(server, id string) {
	_, _ = im.db.Exec(`INSERT OR IGNORE INTO imported (server, asset_id, imported_at) VALUES (?, ?, ?)`,
		server, id, time.Now().Unix())
}

func (im *Importer) setError(msg string) {
	im.mu.Lock()
	im.prog.Error = msg
	im.mu.Unlock()
}

func (im *Importer) fail(err error) {
	im.log.Debug("import asset", "err", err)
	im.count(func(p *Progress) {
		p.Failed++
		p.Error = err.Error()
	})
}

func (im *Importer) count(f func(*Progress)) {
	im.mu.Lock()
	f(&im.prog)
	im.mu.Unlock()
}
