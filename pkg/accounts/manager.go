package accounts

import (
	"context"
	"errors"
	"io"
	"sync"

	"gpix/pkg/gpmc"
	"gpix/pkg/library"
	"gpix/pkg/store"
)

// Runtime is everything gpix needs to serve one account: its Google client, its
// listing cache, and its object-store backend.
type Runtime struct {
	ID    int64
	Label string
	GP    *gpmc.Client
	Lib   *library.Cache
	BE    store.Backend
}

// BuildFunc constructs the runtime for an account. main.go supplies it so this
// package doesn't need to know about cachedb/gpmcstore wiring.
type BuildFunc func(ctx context.Context, id int64, label, authData string) (*Runtime, error)

// Manager owns the linked accounts and which one is active. Runtimes are built
// lazily and cached, so switching back to a previously used account is instant
// (its listing cache is still warm).
type Manager struct {
	store *Store
	build BuildFunc

	mu       sync.RWMutex
	active   *Runtime
	runtimes map[int64]*Runtime
}

// NewManager builds a manager and activates the stored active account.
func NewManager(ctx context.Context, st *Store, build BuildFunc) (*Manager, error) {
	m := &Manager{store: st, build: build, runtimes: map[int64]*Runtime{}}
	id, ok := st.ActiveID(ctx)
	if !ok {
		return nil, errors.New("accounts: no accounts linked")
	}
	if _, err := m.activate(ctx, id); err != nil {
		return nil, err
	}
	return m, nil
}

// activate builds (or reuses) the runtime for id and makes it active.
func (m *Manager) activate(ctx context.Context, id int64) (*Runtime, error) {
	m.mu.RLock()
	rt, cached := m.runtimes[id]
	m.mu.RUnlock()
	if cached {
		m.mu.Lock()
		m.active = rt
		m.mu.Unlock()
		return rt, nil
	}

	list, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}
	label := "Account"
	found := false
	for _, a := range list {
		if a.ID == id {
			label, found = a.Label, true
			break
		}
	}
	if !found {
		return nil, errors.New("accounts: no such account")
	}
	authData, err := m.store.AuthData(ctx, id)
	if err != nil {
		return nil, err
	}
	rt, err = m.build(ctx, id, label, authData)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.runtimes[id] = rt
	m.active = rt
	m.mu.Unlock()
	return rt, nil
}

// Active returns the current account's runtime. It is never nil once the
// manager is constructed.
func (m *Manager) Active() *Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// ActiveID returns the active account's id.
func (m *Manager) ActiveID() int64 {
	rt := m.Active()
	if rt == nil {
		return 0
	}
	return rt.ID
}

// List returns the linked accounts with the active one flagged.
func (m *Manager) List(ctx context.Context) ([]Account, error) {
	list, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}
	activeID := m.ActiveID()
	for i := range list {
		list[i].Active = list[i].ID == activeID
	}
	return list, nil
}

// Add links an account. If it's the first one it also becomes active.
func (m *Manager) Add(ctx context.Context, label, authData string) (Account, error) {
	if _, exists := m.store.FindByAuthData(ctx, authData); exists {
		return Account{}, errors.New("accounts: that account is already linked")
	}
	a, err := m.store.Add(ctx, label, authData)
	if err != nil {
		return Account{}, err
	}
	if a.Active {
		if _, err := m.activate(ctx, a.ID); err != nil {
			return Account{}, err
		}
	}
	return a, nil
}

// SetActive switches the active account (building its runtime if needed).
func (m *Manager) SetActive(ctx context.Context, id int64) error {
	if _, err := m.activate(ctx, id); err != nil {
		return err
	}
	return m.store.SetActive(ctx, id)
}

// Remove unlinks a non-active account and drops its cached runtime.
func (m *Manager) Remove(ctx context.Context, id int64) error {
	if id == m.ActiveID() {
		return errors.New("accounts: switch to another account before removing this one")
	}
	if err := m.store.Remove(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.runtimes, id)
	m.mu.Unlock()
	return nil
}

// Rename relabels an account.
func (m *Manager) Rename(ctx context.Context, id int64, label string) error {
	if err := m.store.Rename(ctx, id, label); err != nil {
		return err
	}
	m.mu.Lock()
	if rt, ok := m.runtimes[id]; ok {
		rt.Label = label
	}
	m.mu.Unlock()
	return nil
}

// AuthData exposes a linked account's credential (used by account→account
// transfers, which read from a non-active account).
func (m *Manager) AuthData(ctx context.Context, id int64) (string, error) {
	return m.store.AuthData(ctx, id)
}

// --- dynamic backend ---

// DynamicBackend is a store.Backend that always forwards to the active
// account's backend. The S3/WebDAV gateways and the importer hold this, so an
// account switch takes effect everywhere without re-wiring them.
type DynamicBackend struct{ m *Manager }

// NewDynamicBackend returns a backend bound to the manager's active account.
func NewDynamicBackend(m *Manager) *DynamicBackend { return &DynamicBackend{m: m} }

func (d *DynamicBackend) be() store.Backend { return d.m.Active().BE }

func (d *DynamicBackend) Name() string { return "gpix-active-account" }

func (d *DynamicBackend) List(ctx context.Context) ([]store.Object, error) {
	return d.be().List(ctx)
}

func (d *DynamicBackend) Stat(ctx context.Context, key string) (store.Object, error) {
	return d.be().Stat(ctx, key)
}

func (d *DynamicBackend) Get(ctx context.Context, key string) (io.ReadCloser, store.Object, error) {
	return d.be().Get(ctx, key)
}

func (d *DynamicBackend) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (store.Object, error) {
	return d.be().Put(ctx, key, r, size, contentType)
}

func (d *DynamicBackend) Delete(ctx context.Context, key string) error {
	return d.be().Delete(ctx, key)
}

var _ store.Backend = (*DynamicBackend)(nil)
