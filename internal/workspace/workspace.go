// Package workspace keeps the named folders terminals run in: a git worktree
// made for a task, or any folder you point at. Workspaces outlive terminals and
// the daemon; terminals do not.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("no such workspace")

type Workspace struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // worktree | folder
	Repo      string    `json:"repo,omitempty"`
	RepoPath  string    `json:"repoPath,omitempty"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	items map[string]*Workspace
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]*Workspace{}}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		var list []*Workspace
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, err
		}
		for _, w := range list {
			s.items[w.Name] = w
		}
	}
	return s, nil
}

func (s *Store) List() []Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Workspace, 0, len(s.items))
	for _, w := range s.items {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) Get(name string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.items[name]
	if !ok {
		return Workspace{}, false
	}
	return *w, true
}

func (s *Store) Add(w Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[w.Name]; exists {
		return fmt.Errorf("workspace %q already exists", w.Name)
	}
	s.items[w.Name] = &w
	return s.saveLocked()
}

func (s *Store) Remove(name string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.items[name]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	delete(s.items, name)
	return *w, s.saveLocked()
}

func (s *Store) saveLocked() error {
	list := make([]*Workspace, 0, len(s.items))
	for _, w := range s.items {
		list = append(list, w)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
