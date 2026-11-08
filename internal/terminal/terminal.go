// Package terminal is the lightweight tmux core: named PTY sessions that
// outlive their clients, with a raw scrollback buffer replayed on attach.
//
// Deliberately absent: windows, panes, layouts, key tables, a config language.
// One terminal is one PTY running one command. Many terminals, many names.
package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

var ErrNotFound = errors.New("no such terminal")

type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// Info is the JSON view of a terminal.
type Info struct {
	Name      string     `json:"name"`
	Command   string     `json:"command"`
	Cwd       string     `json:"cwd"`
	Agent     string     `json:"agent,omitempty"`
	Task      string     `json:"task,omitempty"`
	Repo      string     `json:"repo,omitempty"`
	Workspace string     `json:"workspace,omitempty"`
	Model     string     `json:"model,omitempty"` // requested at launch or by SetModel; hooks may report the real one
	PID       int        `json:"pid"`
	Clients   int        `json:"clients"`
	Size      Size       `json:"size"`
	Alive     bool       `json:"alive"`
	ExitCode  int        `json:"exitCode"`
	StartedAt time.Time  `json:"startedAt"`
	ExitedAt  *time.Time `json:"exitedAt,omitempty"`
	// LastOutputAt lets clients show "quiet for 40s" for shells that have no hooks.
	LastOutputAt time.Time `json:"lastOutputAt"`
}

type Options struct {
	Name      string
	Command   string // run with `shell -lc`; empty starts an interactive shell
	Cwd       string
	Env       map[string]string
	Shell     string // default $SHELL, then /bin/zsh
	Size      Size
	KeepShell bool // after Command exits, exec the shell so the terminal stays usable
	Agent     string
	Task      string
	Repo      string
	Workspace string
	Model     string
}

// Manager owns all terminals of one daemon.
type Manager struct {
	mu         sync.Mutex
	terms      map[string]*Term
	scrollback int
	onChange   func()
}

func NewManager(scrollback int, onChange func()) *Manager {
	if scrollback <= 0 {
		scrollback = 1 << 20
	}
	if onChange == nil {
		onChange = func() {}
	}
	return &Manager{terms: map[string]*Term{}, scrollback: scrollback, onChange: onChange}
}

func (m *Manager) Create(o Options) (*Term, error) {
	if o.Name == "" {
		return nil, errors.New("terminal needs a name")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, exists := m.terms[o.Name]; exists {
		if old.Info().Alive {
			return nil, fmt.Errorf("terminal %q is already running", o.Name)
		}
		delete(m.terms, o.Name)
	}
	t, err := start(m, o)
	if err != nil {
		return nil, err
	}
	m.terms[o.Name] = t
	go m.onChange()
	return t, nil
}

func (m *Manager) Get(name string) (*Term, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.terms[name]
	return t, ok
}

func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.terms))
	for _, t := range m.terms {
		out = append(out, t.Info())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Alive != out[j].Alive {
			return out[i].Alive
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

// Kill ends the process group (HUP, then KILL) and forgets the terminal.
func (m *Manager) Kill(name string) error {
	t, ok := m.Get(name)
	if !ok {
		return ErrNotFound
	}
	info := t.Info()
	if info.Alive {
		_ = syscall.Kill(-info.PID, syscall.SIGHUP)
		select {
		case <-t.exited:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-info.PID, syscall.SIGKILL)
			select {
			case <-t.exited:
			case <-time.After(2 * time.Second):
			}
		}
	}
	m.mu.Lock()
	delete(m.terms, name)
	m.mu.Unlock()
	m.onChange()
	return nil
}

// Term is one PTY plus its scrollback and attached clients.
type Term struct {
	m       *Manager
	mu      sync.Mutex
	info    Info
	f       *os.File
	cmd     *exec.Cmd
	scroll  *ring
	clients map[*Client]struct{}
	exited  chan struct{}
}

func start(m *Manager, o Options) (*Term, error) {
	shell := o.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/zsh"
	}
	var cmd *exec.Cmd
	label := o.Command
	switch {
	case o.Command == "":
		cmd = exec.Command(shell, "-l")
		label = shell
	case o.KeepShell:
		cmd = exec.Command(shell, "-lc", o.Command+`; exec "$SHELL" -l`)
	default:
		cmd = exec.Command(shell, "-lc", o.Command)
	}
	cwd := o.Cwd
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	cmd.Dir = cwd
	env := os.Environ()
	env = setEnv(env, "TERM", "xterm-256color")
	env = setEnv(env, "COLORTERM", "truecolor")
	env = setEnv(env, "SHELL", shell)
	env = setEnv(env, "FLEET_TERM", o.Name)
	keys := make([]string, 0, len(o.Env))
	for k := range o.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = setEnv(env, k, o.Env[k])
	}
	cmd.Env = env
	sz := o.Size
	if sz.Cols == 0 || sz.Rows == 0 {
		sz = Size{Cols: 120, Rows: 36}
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: sz.Rows, Cols: sz.Cols})
	if err != nil {
		return nil, fmt.Errorf("start %q: %w", label, err)
	}
	t := &Term{
		m: m, f: f, cmd: cmd,
		scroll:  newRing(m.scrollback),
		clients: map[*Client]struct{}{},
		exited:  make(chan struct{}),
		info: Info{Name: o.Name, Command: label, Cwd: cwd, Agent: o.Agent, Task: o.Task, Repo: o.Repo, Workspace: o.Workspace, Model: o.Model,
			PID: cmd.Process.Pid, Size: sz, Alive: true, StartedAt: time.Now(), LastOutputAt: time.Now()},
	}
	go t.readLoop()
	return t, nil
}

func (t *Term) Info() Info {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.info
}

// SetModel records the model a client asked the agent to switch to.
func (t *Term) SetModel(m string) {
	t.mu.Lock()
	t.info.Model = m
	t.mu.Unlock()
	go t.m.onChange()
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[@-Z\\-_]|\r`)

// PlainText strips escape sequences and trailing spaces from raw terminal output.
func PlainText(b []byte) []byte {
	out := ansiRe.ReplaceAll(b, nil)
	lines := bytes.Split(out, []byte("\n"))
	for i, l := range lines {
		lines[i] = bytes.TrimRight(l, " \t")
	}
	return bytes.Join(lines, []byte("\n"))
}

// Scrollback returns a copy of the raw output buffer.
func (t *Term) Scrollback() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scroll.Bytes()
}

// Write sends input to the PTY.
func (t *Term) Write(p []byte) error {
	_, err := t.f.Write(p)
	return err
}

// Attach registers a client and returns it with the scrollback to replay.
func (t *Term) Attach(sz Size) (*Client, []byte) {
	c := &Client{t: t, out: make(chan []byte, 1024), exit: make(chan int, 1), size: sz}
	t.mu.Lock()
	t.clients[c] = struct{}{}
	t.info.Clients = len(t.clients)
	snap := t.scroll.Bytes()
	alive, code := t.info.Alive, t.info.ExitCode
	t.mu.Unlock()
	if !alive {
		c.notifyExit(code)
	}
	t.applySize(true)
	go t.m.onChange()
	return c, snap
}

func (t *Term) detach(c *Client) {
	t.mu.Lock()
	if _, ok := t.clients[c]; !ok {
		t.mu.Unlock()
		return
	}
	delete(t.clients, c)
	t.info.Clients = len(t.clients)
	t.mu.Unlock()
	close(c.out)
	t.applySize(false)
	go t.m.onChange()
}

// applySize sets the PTY to the smallest attached client. With nudge, it
// shrinks by one column and grows back so full-screen TUIs repaint for the
// newcomer; raw scrollback replay alone leaves alt-screen apps half drawn.
func (t *Term) applySize(nudge bool) {
	t.mu.Lock()
	var sz Size
	for c := range t.clients {
		if c.size.Cols == 0 || c.size.Rows == 0 {
			continue
		}
		if sz.Cols == 0 || c.size.Cols < sz.Cols {
			sz.Cols = c.size.Cols
		}
		if sz.Rows == 0 || c.size.Rows < sz.Rows {
			sz.Rows = c.size.Rows
		}
	}
	if sz.Cols == 0 {
		t.mu.Unlock()
		return
	}
	t.info.Size = sz
	alive, f := t.info.Alive, t.f
	t.mu.Unlock()
	if !alive {
		return
	}
	if nudge && sz.Cols > 1 {
		go func() {
			_ = pty.Setsize(f, &pty.Winsize{Rows: sz.Rows, Cols: sz.Cols - 1})
			time.Sleep(80 * time.Millisecond)
			t.mu.Lock()
			cur := t.info.Size // a resize may have landed meanwhile; never restore a stale size
			t.mu.Unlock()
			_ = pty.Setsize(f, &pty.Winsize{Rows: cur.Rows, Cols: cur.Cols})
		}()
		return
	}
	_ = pty.Setsize(f, &pty.Winsize{Rows: sz.Rows, Cols: sz.Cols})
}

func (t *Term) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.f.Read(buf)
		if n > 0 {
			t.broadcast(buf[:n])
		}
		if err != nil {
			break
		}
	}
	err := t.cmd.Wait()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
	}
	now := time.Now()
	t.mu.Lock()
	t.info.Alive = false
	t.info.ExitCode = code
	t.info.ExitedAt = &now
	clients := make([]*Client, 0, len(t.clients))
	for c := range t.clients {
		clients = append(clients, c)
	}
	t.mu.Unlock()
	_ = t.f.Close()
	close(t.exited)
	for _, c := range clients {
		c.notifyExit(code)
	}
	t.m.onChange()
}

func (t *Term) broadcast(p []byte) {
	chunk := append([]byte(nil), p...)
	t.mu.Lock()
	t.scroll.Write(chunk)
	t.info.LastOutputAt = time.Now()
	for c := range t.clients {
		select {
		case c.out <- chunk:
		default: // slow client: drop; it will catch up from the live stream
		}
	}
	t.mu.Unlock()
}

// Client is one attached viewer/typist.
type Client struct {
	t    *Term
	out  chan []byte
	exit chan int
	size Size
	once sync.Once
}

func (c *Client) Output() <-chan []byte { return c.out }
func (c *Client) Exited() <-chan int    { return c.exit }
func (c *Client) Write(p []byte) error  { return c.t.Write(p) }
func (c *Client) Close()                { c.t.detach(c) }

func (c *Client) Resize(sz Size) {
	c.t.mu.Lock()
	c.size = sz
	c.t.mu.Unlock()
	c.t.applySize(false)
}

func (c *Client) notifyExit(code int) {
	c.once.Do(func() { c.exit <- code })
}

type ring struct {
	buf bytes.Buffer
	max int
}

func newRing(max int) *ring { return &ring{max: max} }

func (r *ring) Write(p []byte) {
	r.buf.Write(p)
	if over := r.buf.Len() - r.max; over > 0 {
		r.buf.Next(over)
	}
}

func (r *ring) Bytes() []byte { return append([]byte(nil), r.buf.Bytes()...) }

func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	out := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return append(out, prefix+val)
}
