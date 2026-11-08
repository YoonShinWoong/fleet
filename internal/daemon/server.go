// Package daemon is the control plane: PTY terminals, hook ingestion, session
// state, a WebSocket attach protocol, SSE for live views, and the web client.
package daemon

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/YoonShinWoong/fleet/internal/config"
	"github.com/YoonShinWoong/fleet/internal/hooks"
	"github.com/YoonShinWoong/fleet/internal/launch"
	"github.com/YoonShinWoong/fleet/internal/state"
	"github.com/YoonShinWoong/fleet/internal/terminal"
	"github.com/YoonShinWoong/fleet/internal/version"
	"github.com/YoonShinWoong/fleet/internal/workspace"
)

//go:embed web
var webFS embed.FS

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type Server struct {
	cfg      *config.Config
	store    *state.Store
	terms    *terminal.Manager
	ws       *workspace.Store
	launcher *launch.Launcher
	srv      *http.Server

	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

// Snapshot is what live clients receive: every terminal and every agent session.
type Snapshot struct {
	Terms      []terminal.Info       `json:"terms"`
	Sessions   []state.Session       `json:"sessions"`
	Workspaces []workspace.Workspace `json:"workspaces"`
}

func New(cfg *config.Config, store *state.Store) (*Server, error) {
	ws, err := workspace.Open(config.WorkspacesPath())
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, store: store, ws: ws, subs: map[chan struct{}]struct{}{}}
	s.terms = terminal.NewManager(cfg.ScrollbackBytes, s.changed)
	s.launcher = launch.New(cfg, s.terms, ws)
	ch, _ := store.Subscribe()
	go func() {
		for range ch {
			s.changed()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/events/{agent}", s.postEvent)
	mux.HandleFunc("GET /v1/sessions", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.store.List()) })
	mux.HandleFunc("GET /v1/sessions/{id}", s.getSession)
	mux.HandleFunc("GET /v1/terms", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.terms.List()) })
	mux.HandleFunc("POST /v1/terms", s.createTerm)
	mux.HandleFunc("GET /v1/terms/{name}", s.getTerm)
	mux.HandleFunc("DELETE /v1/terms/{name}", s.killTerm)
	mux.HandleFunc("GET /v1/terms/{name}/ws", s.attachWS)
	mux.HandleFunc("POST /v1/terms/{name}/send", s.sendTerm)
	mux.HandleFunc("POST /v1/terms/{name}/model", s.setModel)
	mux.HandleFunc("POST /v1/terms/{name}/handoff", s.handoff)
	mux.HandleFunc("GET /v1/terms/{name}/screen", s.screen)
	mux.HandleFunc("POST /v1/tasks", s.newTask)
	mux.HandleFunc("GET /v1/workspaces", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.ws.List()) })
	mux.HandleFunc("POST /v1/workspaces", s.createWorkspace)
	mux.HandleFunc("DELETE /v1/workspaces/{name}", s.removeWorkspace)
	mux.HandleFunc("GET /v1/agents", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.launcher.Agents()) })
	mux.HandleFunc("GET /v1/repos", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.launcher.Repos()) })
	mux.HandleFunc("GET /v1/search", s.search)
	mux.HandleFunc("GET /v1/stream", s.stream)
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServerFS(sub))

	s.srv = &http.Server{Addr: cfg.Addr, Handler: logging(s.auth(mux)), ReadHeaderTimeout: 5 * time.Second}
	return s, nil
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var req launch.WorkspaceRequest
	if err := readJSON(r, &req, 1<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, created, err := s.launcher.EnsureWorkspace(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.changed()
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, ws)
}

func (s *Server) removeWorkspace(w http.ResponseWriter, r *http.Request) {
	err := s.launcher.RemoveWorkspace(r.PathValue("name"), r.URL.Query().Get("delete") != "")
	switch {
	case errors.Is(err, workspace.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		s.changed()
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) ListenAndServe() error {
	log.Printf("fleet %s listening on http://%s (config: %s)", version.Version, s.cfg.Addr, config.Path())
	return s.srv.ListenAndServe()
}

// changed wakes every SSE subscriber. Non-blocking: a slow viewer just coalesces.
func (s *Server) changed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Server) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

func (s *Server) snapshot() Snapshot {
	return Snapshot{Terms: s.terms.List(), Sessions: s.store.List(), Workspaces: s.ws.List()}
}

// auth guards /v1 for non-loopback callers with the config token, via
// Authorization: Bearer or ?token= (EventSource and WebSocket cannot set headers).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") || isLoopback(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		if s.cfg.Token == "" {
			http.Error(w, "daemon has no token; refusing non-loopback access", http.StatusForbidden)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || got == r.Header.Get("Authorization") {
			got = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version.Version, "terms": len(s.terms.List()), "sessions": len(s.store.List())})
}

func (s *Server) postEvent(w http.ResponseWriter, r *http.Request) {
	var env hooks.Envelope
	if err := readJSON(r, &env, 8<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	env.Context.Agent = r.PathValue("agent")
	sess, err := hooks.Apply(s.store, env)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusAccepted, sess)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.store.Find(r.PathValue("id"))
	if !ok {
		http.Error(w, "no such session", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

type createTermRequest struct {
	Name      string            `json:"name"`
	Workspace string            `json:"workspace"` // when set, cwd and env come from the workspace
	Agent     string            `json:"agent"`     // shell (default) or a configured agent; needs Workspace
	Model     string            `json:"model"`     // agent model at launch
	Prompt    string            `json:"prompt"`    // agent's first prompt
	Cwd       string            `json:"cwd"`
	Command   string            `json:"command"`
	Env       map[string]string `json:"env"`
	KeepShell bool              `json:"keepShell"`
	Cols      uint16            `json:"cols"`
	Rows      uint16            `json:"rows"`
}

func (s *Server) createTerm(w http.ResponseWriter, r *http.Request) {
	var req createTermRequest
	if err := readJSON(r, &req, 1<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Workspace != "" {
		info, err := s.launcher.Open(launch.OpenRequest{Workspace: req.Workspace, Agent: req.Agent, Name: req.Name, Model: req.Model, Prompt: req.Prompt, Cols: req.Cols, Rows: req.Rows})
		if err != nil {
			code := http.StatusConflict
			if errors.Is(err, workspace.ErrNotFound) {
				code = http.StatusNotFound
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, http.StatusCreated, info)
		return
	}
	if !nameRe.MatchString(req.Name) {
		http.Error(w, "name must match "+nameRe.String(), http.StatusBadRequest)
		return
	}
	t, err := s.terms.Create(terminal.Options{
		Name: req.Name, Command: req.Command, Cwd: config.Expand(req.Cwd), Env: req.Env,
		Shell: s.cfg.Shell, KeepShell: req.KeepShell, Size: terminal.Size{Cols: req.Cols, Rows: req.Rows},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, t.Info())
}

func (s *Server) getTerm(w http.ResponseWriter, r *http.Request) {
	t, ok := s.terms.Get(r.PathValue("name"))
	if !ok {
		http.Error(w, "no such terminal", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, t.Info())
}

func (s *Server) killTerm(w http.ResponseWriter, r *http.Request) {
	if err := s.terms.Kill(r.PathValue("name")); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, terminal.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sendTerm(w http.ResponseWriter, r *http.Request) {
	t, ok := s.terms.Get(r.PathValue("name"))
	if !ok {
		http.Error(w, "no such terminal", http.StatusNotFound)
		return
	}
	var body struct {
		Text  string `json:"text"`
		Enter *bool  `json:"enter"`
	}
	if err := readJSON(r, &body, 1<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data := []byte(body.Text)
	if body.Enter == nil || *body.Enter {
		data = append(data, '\r')
	}
	if err := t.Write(data); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	if err := readJSON(r, &body, 1<<20); err != nil || body.Model == "" {
		http.Error(w, "need {\"model\": \"...\"}", http.StatusBadRequest)
		return
	}
	if err := s.launcher.SetModel(r.PathValue("name"), body.Model); err != nil {
		code := http.StatusConflict
		if errors.Is(err, terminal.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handoff(w http.ResponseWriter, r *http.Request) {
	var req launch.HandoffRequest
	if err := readJSON(r, &req, 1<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.From = r.PathValue("name")
	res, err := s.launcher.Handoff(req)
	if err != nil {
		code := http.StatusConflict
		if errors.Is(err, terminal.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]any{"error": err.Error(), "result": res})
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// screen returns scrollback. ?plain=1 strips escapes, ?lines=N keeps the tail.
func (s *Server) screen(w http.ResponseWriter, r *http.Request) {
	t, ok := s.terms.Get(r.PathValue("name"))
	if !ok {
		http.Error(w, "no such terminal", http.StatusNotFound)
		return
	}
	out := t.Scrollback()
	if r.URL.Query().Get("plain") != "" {
		out = terminal.PlainText(out)
	}
	if n, _ := strconv.Atoi(r.URL.Query().Get("lines")); n > 0 {
		lines := strings.Split(string(out), "\n")
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
		out = []byte(strings.Join(lines, "\n"))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(out)
}

func (s *Server) newTask(w http.ResponseWriter, r *http.Request) {
	var req launch.Request
	if err := readJSON(r, &req, 1<<20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.launcher.NewTask(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "result": res})
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// attachWS is the terminal wire protocol. Binary frames carry PTY bytes both
// ways; text frames are JSON control: {"t":"resize","cols":..,"rows":..} in,
// {"t":"exit","code":..} out. The first frame to the client replays scrollback.
func (s *Server) attachWS(w http.ResponseWriter, r *http.Request) {
	t, ok := s.terms.Get(r.PathValue("name"))
	if !ok {
		http.Error(w, "no such terminal", http.StatusNotFound)
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(4 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client, snap := t.Attach(terminal.Size{Cols: uint16(cols), Rows: uint16(rows)})
	defer client.Close()
	if len(snap) > 0 {
		if err := c.Write(ctx, websocket.MessageBinary, snap); err != nil {
			return
		}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-client.Output():
				if !ok {
					cancel()
					return
				}
				if err := c.Write(ctx, websocket.MessageBinary, chunk); err != nil {
					cancel()
					return
				}
			case code := <-client.Exited():
				msg, _ := json.Marshal(map[string]any{"t": "exit", "code": code})
				_ = c.Write(ctx, websocket.MessageText, msg)
				_ = c.Close(websocket.StatusNormalClosure, "exited")
				cancel()
				return
			}
		}
	}()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			_ = client.Write(data)
			continue
		}
		var ctl struct {
			T    string `json:"t"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if json.Unmarshal(data, &ctl) == nil && ctl.T == "resize" {
			client.Resize(terminal.Size{Cols: ctl.Cols, Rows: ctl.Rows})
		}
	}
}

type searchHit struct {
	Term      string   `json:"term"`
	Workspace string   `json:"workspace,omitempty"`
	Agent     string   `json:"agent,omitempty"`
	Count     int      `json:"count"`
	Lines     []string `json:"lines"` // most recent matching lines, newest last
}

// search scans every terminal's plain scrollback for a case-insensitive substring.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len([]rune(q)) < 2 {
		http.Error(w, "q must be at least 2 characters", http.StatusBadRequest)
		return
	}
	const keep = 3
	out := []searchHit{}
	for _, info := range s.terms.List() {
		t, ok := s.terms.Get(info.Name)
		if !ok {
			continue
		}
		h := searchHit{Term: info.Name, Workspace: info.Workspace, Agent: info.Agent, Lines: []string{}}
		for _, line := range strings.Split(string(terminal.PlainText(t.Scrollback())), "\n") {
			if !strings.Contains(strings.ToLower(line), q) {
				continue
			}
			h.Count++
			line = strings.TrimSpace(line)
			if rs := []rune(line); len(rs) > 200 {
				line = string(rs[:200]) + "…"
			}
			h.Lines = append(h.Lines, line)
			if len(h.Lines) > keep {
				h.Lines = h.Lines[1:]
			}
		}
		if h.Count > 0 {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	writeJSON(w, http.StatusOK, out)
}

// stream sends the full snapshot on connect, after every change (coalesced), and every 10s.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	ch, cancel := s.subscribe()
	defer cancel()
	send := func() {
		b, _ := json.Marshal(s.snapshot())
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", b)
		fl.Flush()
	}
	send()
	tick := time.NewTicker(10 * time.Second) // periodic snapshot doubles as keepalive and refreshes ages
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			time.Sleep(40 * time.Millisecond)
			select {
			case <-ch:
			default:
			}
			send()
		case <-tick.C:
			send()
		}
	}
}

func readJSON(r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty body")
		}
		return fmt.Errorf("bad json: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(c int) { sw.code = c; sw.ResponseWriter.WriteHeader(c) }
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// logging records mutating calls; GETs and the long-lived streams stay quiet.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		sw := &statusWriter{ResponseWriter: w, code: 200}
		t := time.Now()
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(t).Round(time.Millisecond))
	})
}
