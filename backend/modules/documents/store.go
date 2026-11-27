package documents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound is returned when a stored object does not exist.
var ErrNotFound = errors.New("documents: stored file not found")

// Store keeps document bytes outside the database. Files are private: the
// store is never exposed directly, only through short-lived links (spec §4.4).
type Store interface {
	Put(key string, data []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
}

// MemoryStore keeps files in memory. For tests.
type MemoryStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{files: map[string][]byte{}} }

func (m *MemoryStore) Put(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[key] = append([]byte(nil), data...)
	return nil
}

func (m *MemoryStore) Get(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), b...), nil
}

// Delete removes a file. A missing file is not an error, so purge can retry.
func (m *MemoryStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	return nil
}

// Has reports whether a key is stored. For tests.
func (m *MemoryStore) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[key]
	return ok
}

// LocalStore keeps files in a directory on this machine. Development only;
// production storage is chosen with hosting (plan Task 34 note).
type LocalStore struct{ dir string }

// NewLocalStore returns a store rooted at dir, creating it if needed.
func NewLocalStore(dir string) (*LocalStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("documents: store dir: %w", err)
	}
	return &LocalStore{dir: dir}, nil
}

func (l *LocalStore) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if clean == "/" {
		return "", errors.New("documents: empty key")
	}
	return filepath.Join(l.dir, clean), nil
}

func (l *LocalStore) Put(key string, data []byte) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("documents: mkdir: %w", err)
	}
	return os.WriteFile(p, data, 0o600)
}

func (l *LocalStore) Get(key string) ([]byte, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (l *LocalStore) Delete(key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
