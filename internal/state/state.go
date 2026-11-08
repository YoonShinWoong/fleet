package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusUnknown    Status = "unknown"
	StatusWorking    Status = "working"
	StatusNeedsInput Status = "needs_input"
	StatusIdle       Status = "idle"
	StatusEnded      Status = "ended"
	StatusError      Status = "error"
)

// Session is one agent conversation as seen through its hook events.
type Session struct {
	ID          string    `json:"id"`
	Agent       string    `json:"agent"`
	Task        string    `json:"task,omitempty"`
	Repo        string    `json:"repo,omitempty"`
	Cwd         string    `json:"cwd,omitempty"`
	Host        string    `json:"host,omitempty"`
	Term        string    `json:"term,omitempty"`
	Model       string    `json:"model,omitempty"`
	Status      Status    `json:"status"`
	LastEvent   string    `json:"lastEvent,omitempty"`
	LastTool    string    `json:"lastTool,omitempty"`
	LastMessage string    `json:"lastMessage,omitempty"`
	Subagents   int       `json:"subagents"`
	Events      int       `json:"events"`
	StartedAt   time.Time `json:"startedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Label is the human name: task, else cwd basename, else id prefix.
func (s Session) Label() string {
	if s.Task != "" {
		return s.Task
	}
	if s.Cwd != "" {
		return filepath.Base(s.Cwd)
	}
	if len(s.ID) > 8 {
		return s.ID[:8]
	}
	return s.ID
}

// Store keeps sessions in memory and mirrors them to a JSON file.
type Store struct {
	mu       sync.RWMutex
	path     string
	sessions map[string]*Session
	subs     map[chan Session]struct{}
}

func Open(path string) (*Store, error) {
	st := &Store{path: path, sessions: map[string]*Session{}, subs: map[chan Session]struct{}{}}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		var list []*Session
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, err
		}
		for _, s := range list {
			st.sessions[s.ID] = s
		}
	}
	return st, nil
}

// Update applies fn to the session (creating it if needed), persists, and notifies subscribers.
func (st *Store) Update(id string, fn func(*Session)) Session {
	st.mu.Lock()
	s, ok := st.sessions[id]
	if !ok {
		s = &Session{ID: id, Status: StatusUnknown}
		st.sessions[id] = s
	}
	fn(s)
	snap := *s
	_ = st.saveLocked()
	subs := make([]chan Session, 0, len(st.subs))
	for ch := range st.subs {
		subs = append(subs, ch)
	}
	st.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default:
		}
	}
	return snap
}

func (st *Store) List() []Session {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]Session, 0, len(st.sessions))
	for _, s := range st.sessions {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (st *Store) Get(id string) (Session, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	s, ok := st.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

// Find matches exact id, id prefix, or label; newest first.
func (st *Store) Find(q string) (Session, bool) {
	for _, s := range st.List() {
		if s.ID == q || strings.HasPrefix(s.ID, q) || s.Label() == q {
			return s, true
		}
	}
	return Session{}, false
}

// Subscribe returns a channel of session snapshots and a cancel func.
func (st *Store) Subscribe() (<-chan Session, func()) {
	ch := make(chan Session, 32)
	st.mu.Lock()
	st.subs[ch] = struct{}{}
	st.mu.Unlock()
	return ch, func() {
		st.mu.Lock()
		delete(st.subs, ch)
		st.mu.Unlock()
	}
}

func (st *Store) saveLocked() error {
	list := make([]*Session, 0, len(st.sessions))
	for _, s := range st.sessions {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}
