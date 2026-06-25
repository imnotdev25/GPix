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
	Total      int       `json:"total"`
	Done       int       `json:"done"`
	Skipped    int       `json:"skipped"`
	Failed     int       `json:"failed"`
	Current    string    `json:"current"`
	Error      string    `json:"error"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

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

// Start kicks off an import in the background. It returns an error if one is
// already running.
func (im *Importer) Start(serverURL, apiKey string) error {
	im.mu.Lock()
	if im.prog.Running {
		im.mu.Unlock()
		return errors.New("an import is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	im.cancel = cancel
	im.prog = Progress{Running: true, Server: serverURL, StartedAt: time.Now()}
	im.mu.Unlock()

	client := NewClient(serverURL, apiKey)
	go im.run(ctx, client, serverURL)
	return nil
}

func (im *Importer) run(ctx context.Context, client *Client, serverURL string) {
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

	var prevFirstID string
	for page := 1; ; page++ {
		if ctx.Err() != nil {
			return
		}
		items, total, err := client.SearchPage(ctx, page)
		if err != nil {
			im.setError(err.Error())
			return
		}
		if page == 1 {
			im.mu.Lock()
			im.prog.Total = total
			im.mu.Unlock()
		}
		if len(items) == 0 {
			return
		}
		// Guard against servers that ignore the page parameter and keep returning
		// the same first page (which would loop forever).
		if items[0].ID == prevFirstID {
			return
		}
		prevFirstID = items[0].ID

		for _, a := range items {
			if ctx.Err() != nil {
				return
			}
			im.processAsset(ctx, client, serverURL, a)
		}
		if len(items) < PageSize {
			return // last page
		}
	}
}

func (im *Importer) processAsset(ctx context.Context, client *Client, serverURL string, a Asset) {
	im.mu.Lock()
	im.prog.Current = a.OriginalFileName
	im.mu.Unlock()

	if im.alreadyImported(serverURL, a.ID) {
		im.count(func(p *Progress) { p.Skipped++ })
		return
	}

	rc, err := client.Download(ctx, a.ID)
	if err != nil {
		im.fail(err)
		return
	}
	defer rc.Close()

	name := a.OriginalFileName
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
