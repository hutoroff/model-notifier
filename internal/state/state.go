// Package state persists which items have already been seen per source.
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type State struct {
	path string
	mu   sync.Mutex
	seen map[string]map[string]time.Time // source → item key → first seen
}

// Load reads path; a missing file yields an empty state.
func Load(path string) (*State, error) {
	s := &State{path: path, seen: map[string]map[string]time.Time{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.seen); err != nil {
		return nil, err
	}
	if s.seen == nil {
		s.seen = map[string]map[string]time.Time{}
	}
	return s, nil
}

// Has reports whether src has been seeded (even with zero items).
func (s *State) Has(src string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[src]
	return ok
}

func (s *State) Seen(src, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[src][key]
	return ok
}

// Mark records src as seeded without adding items.
func (s *State) Mark(src string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mark(src)
}

func (s *State) mark(src string) map[string]time.Time {
	m, ok := s.seen[src]
	if !ok {
		m = map[string]time.Time{}
		s.seen[src] = m
	}
	return m
}

func (s *State) Add(src, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.mark(src)
	if _, ok := m[key]; !ok {
		m[key] = time.Now().UTC()
	}
}

// Save writes the state atomically (tmp file + rename in the same dir).
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(s.seen, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after successful rename
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
