package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/YoonShinWoong/fleet/internal/config"
	"github.com/YoonShinWoong/fleet/internal/state"
	"github.com/YoonShinWoong/fleet/internal/terminal"
)

// savedTerm is one line of ~/.fleet/terminals.json: enough to bring a terminal
// back after the daemon restarts. Agents come back on their own session via the
// agent's resumeCommand; shells come back fresh with their last output shown.
type savedTerm struct {
	terminal.Persisted
	SessionID string    `json:"sessionId,omitempty"`
	Tail      string    `json:"tail,omitempty"`
	SavedAt   time.Time `json:"savedAt"`
}

const tailLines = 200

var persistMu sync.Mutex

// persist writes every live terminal to disk. It runs on changes (debounced),
// every 30s, and on shutdown, so a crash loses at most half a minute of tail.
func (s *Server) persist() {
	persistMu.Lock()
	defer persistMu.Unlock()
	now := time.Now()
	var out []savedTerm
	for _, p := range s.terms.Persisted() {
		st := savedTerm{Persisted: p, SavedAt: now}
		if p.Agent != "" {
			st.SessionID = s.sessionFor(p.Name)
		}
		if t, ok := s.terms.Get(p.Name); ok {
			st.Tail = lastLines(string(terminal.PlainText(t.Scrollback())), tailLines)
		}
		out = append(out, st)
	}
	if out == nil {
		out = []savedTerm{}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	path := config.TerminalsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, path)
	}
}

// sessionFor is the newest agent session id hooks have bound to this terminal.
func (s *Server) sessionFor(term string) string {
	var best state.Session
	for _, sess := range s.store.List() {
		if sess.Term != term || sess.Agent == "" || strings.HasPrefix(sess.ID, "osc:") {
			continue
		}
		if best.ID == "" || sess.UpdatedAt.After(best.UpdatedAt) {
			best = sess
		}
	}
	return best.ID
}

// restore recreates the terminals saved by the previous daemon.
func (s *Server) restore() {
	b, err := os.ReadFile(config.TerminalsPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("restore: %v", err)
		}
		return
	}
	var saved []savedTerm
	if err := json.Unmarshal(b, &saved); err != nil {
		log.Printf("restore: terminals.json unreadable, keeping it aside: %v", err)
		_ = os.Rename(config.TerminalsPath(), config.TerminalsPath()+".bad-"+time.Now().Format("20060102-150405"))
		return
	}
	for _, st := range saved {
		opts := terminal.Options{
			Name: st.Name, Cwd: st.Cwd, Shell: st.Shell, KeepShell: st.KeepShell, Env: st.Env,
			Agent: st.Agent, Task: st.Task, Repo: st.Repo, Workspace: st.Workspace, Model: st.Model, Size: st.Size, Restored: true,
		}
		if opts.Shell == "" {
			opts.Shell = s.cfg.Shell
		}
		var banner strings.Builder
		fmt.Fprintf(&banner, "\x1b[2m[fleet] restored after a daemon restart (saved %s)\x1b[0m\r\n", st.SavedAt.Format("15:04:05"))
		how := "shell"
		switch {
		case st.Agent != "" && st.SessionID != "" && s.cfg.Agents[st.Agent].ResumeCommand != "":
			opts.Command = strings.ReplaceAll(s.cfg.Agents[st.Agent].ResumeCommand, "{session}", terminal.ShellQuote(st.SessionID))
			opts.KeepShell = true
			how = "resume " + st.Agent + " " + short(st.SessionID)
			fmt.Fprintf(&banner, "\x1b[2m[fleet] resuming %s session %s\x1b[0m\r\n", st.Agent, short(st.SessionID))
		case st.Agent != "":
			opts.Command = st.Command
			opts.KeepShell = true
			how = "restart " + st.Agent + " (no session id)"
			fmt.Fprintf(&banner, "\x1b[2m[fleet] no session id was recorded for %s; starting it fresh\x1b[0m\r\n", st.Agent)
		default:
			opts.Command = ""
		}
		if st.Tail != "" {
			banner.WriteString("\x1b[2m--- previous output ---\x1b[0m\r\n")
			banner.WriteString(strings.ReplaceAll(st.Tail, "\n", "\r\n"))
			banner.WriteString("\r\n\x1b[2m--- end of previous output ---\x1b[0m\r\n")
		}
		opts.Preload = []byte(banner.String())
		if _, err := s.terms.Create(opts); err != nil {
			log.Printf("restore %s: %v", st.Name, err)
			continue
		}
		log.Printf("restored %s (%s)", st.Name, how)
	}
}

// Shutdown saves terminals, hangs up their processes and stops the HTTP server.
func (s *Server) Shutdown() {
	s.persist()
	for _, info := range s.terms.List() {
		if info.Alive {
			_ = syscall.Kill(-info.PID, syscall.SIGHUP)
		}
	}
	_ = s.srv.Close()
}

func lastLines(text string, n int) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
