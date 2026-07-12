package immichimport

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
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

// Start kicks off an import in the background using `workers` parallel
// transfers (clamped to 1..maxWorkers). It returns an error if one is already
// running.
func (im *Importer) Start(serverURL, apiKey string, workers int) error {
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
	im.prog = Progress{Running: true, Server: serverURL, Workers: workers, StartedAt: time.Now()}
	im.mu.Unlock()

	client := NewClient(serverURL, apiKey)
	go im.run(ctx, client, serverURL, workers)
	return nil
}

func (im *Importer) run(ctx context.Context, client *Client, serverURL string, workers int) {
	defer func() {
		im.mu.Lock()
		im.prog.Running = false
		im.prog.FinishedAt = time.Now()
		im.cancel = nil
		im.mu.Unlock()
	}()

	if err := client.Ping(ctx); err != nil {
		im.setError("connect: " + err.Error())
		return
	}

	// Enumerate the whole library via timeline buckets (robust across Immich
	// versions, unlike search pagination).
	buckets, err := client.Buckets(ctx)
	if err != nil {
		im.setError("list library: " + err.Error())
		return
	}
	total := 0
	for _, b := range buckets {
		total += b.Count
	}
	im.mu.Lock()
	im.prog.Total = total
	im.mu.Unlock()

	// Worker pool: the producer walks each bucket and feeds assets to `workers`
	// goroutines that download + upload in parallel.
	ch := make(chan Asset, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for a := range ch {
				if ctx.Err() != nil {
					return
				}
				im.processAsset(ctx, client, serverURL, a)
			}
		}()
	}

	stop := false
	for _, b := range buckets {
		if ctx.Err() != nil {
			break
		}
		assets, aerr := client.BucketAssets(ctx, b.TimeBucket)
		if aerr != nil {
			im.setError(aerr.Error())
			continue // skip this bucket, keep importing the rest
		}
		for _, a := range assets {
			select {
			case ch <- a:
			case <-ctx.Done():
				stop = true
			}
			if stop {
				break
			}
		}
		if stop {
			break
		}
	}
	close(ch)
	wg.Wait()
}

func (im *Importer) processAsset(ctx context.Context, client *Client, serverURL string, a Asset) {
	im.mu.Lock()
	im.prog.Current = a.OriginalFileName
	im.mu.Unlock()

	if im.alreadyImported(serverURL, a.ID) {
		im.count(func(p *Progress) { p.Skipped++ })
		return
	}

	rc, fname, err := client.Download(ctx, a.ID)
	if err != nil {
		im.fail(err)
		return
	}
	defer rc.Close()

	name := a.OriginalFileName
	if name == "" {
		name = fname // from Content-Disposition (columnar timeline has no filename)
	}
	if name == "" {
		name = a.ID
	}
	if _, err := im.be.Put(ctx, name, rc, -1, ""); err != nil {
		im.fail(err)
		return
	}
	im.markImported(serverURL, a.ID)
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
