package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/YoonShinWoong/fleet/internal/config"
	"github.com/YoonShinWoong/fleet/internal/state"
	"github.com/YoonShinWoong/fleet/internal/terminal"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	t.Setenv("FLEET_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Shell = "/bin/sh"
	store, err := state.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.srv.Handler)
	t.Cleanup(ts.Close)
	return s, ts
}

func TestAttachProtocol(t *testing.T) {
	s, ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/terms", "application/json", strings.NewReader(`{"name":"ws1","command":"cat","cols":80,"rows":24}`))
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("create: %v %v", err, resp.Status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/v1/terms/ws1/ws?cols=80&rows=24", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	if err := c.Write(ctx, websocket.MessageBinary, []byte("ping-ws\n")); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for bytes.Count(got, []byte("ping-ws")) < 2 { // tty echo + cat
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v (got %q)", err, got)
		}
		if typ == websocket.MessageBinary {
			got = append(got, data...)
		}
	}

	if err := c.Write(ctx, websocket.MessageText, []byte(`{"t":"resize","cols":100,"rows":30}`)); err != nil {
		t.Fatal(err)
	}
	term, _ := s.terms.Get("ws1")
	deadline := time.Now().Add(2 * time.Second)
	for term.Info().Size != (terminal.Size{Cols: 100, Rows: 30}) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if sz := term.Info().Size; sz != (terminal.Size{Cols: 100, Rows: 30}) {
		t.Fatalf("resize not applied: %+v", sz)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/terms/ws1", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("kill: %v %v", err, resp.Status)
	}
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("expected exit frame, got %v", err)
		}
		if typ == websocket.MessageText {
			var m struct {
				T string `json:"t"`
			}
			_ = json.Unmarshal(data, &m)
			if m.T == "exit" {
				return
			}
		}
	}
}

func TestHookEventBindsToTerminal(t *testing.T) {
	_, ts := newTestServer(t)
	env := `{"context":{"term":"smoke","host":"h"},"payload":{"session_id":"abc","hook_event_name":"PermissionRequest","tool_name":"Bash","cwd":"/tmp"}}`
	resp, err := http.Post(ts.URL+"/v1/events/claude", "application/json", strings.NewReader(env))
	if err != nil || resp.StatusCode != 202 {
		t.Fatalf("event: %v %v", err, resp.Status)
	}
	var sess state.Session
	_ = json.NewDecoder(resp.Body).Decode(&sess)
	if sess.Term != "smoke" || sess.Status != state.StatusNeedsInput || sess.Agent != "claude" {
		t.Fatalf("session: %+v", sess)
	}
}

func TestAuthOnlyOffLoopback(t *testing.T) {
	t.Setenv("FLEET_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Token = "secret"
	store, _ := state.Open(filepath.Join(t.TempDir(), "state.json"))
	s, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/terms", nil)
	req.RemoteAddr = "10.0.0.5:4444"
	s.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("remote without token: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req.Header.Set("Authorization", "Bearer secret")
	s.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("remote with token: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/terms?token=secret", nil)
	req.RemoteAddr = "10.0.0.5:4444"
	s.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("remote with query token: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/terms", nil)
	req.RemoteAddr = "127.0.0.1:4444"
	s.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("loopback without token: %d", rec.Code)
	}
}

func TestWorkspaceThenTerminal(t *testing.T) {
	_, ts := newTestServer(t)
	dir := t.TempDir()
	resp, err := http.Post(ts.URL+"/v1/workspaces", "application/json", strings.NewReader(`{"name":"notes","path":"`+dir+`"}`))
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("workspace: %v %v", err, resp.Status)
	}
	resp, err = http.Post(ts.URL+"/v1/terms", "application/json", strings.NewReader(`{"workspace":"notes","agent":"shell","cols":80,"rows":24}`))
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("term: %v %v", err, resp.Status)
	}
	var info terminal.Info
	_ = json.NewDecoder(resp.Body).Decode(&info)
	if info.Name != "notes-shell" || info.Workspace != "notes" || info.Cwd != dir {
		t.Fatalf("info: %+v", info)
	}
	resp, _ = http.Post(ts.URL+"/v1/terms", "application/json", strings.NewReader(`{"workspace":"nope","agent":"shell"}`))
	if resp.StatusCode != 404 {
		t.Fatalf("missing workspace: %v", resp.Status)
	}
	resp, _ = http.Get(ts.URL + "/v1/agents")
	var agents []map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&agents)
	if len(agents) < 3 || agents[0]["name"] != "shell" {
		t.Fatalf("agents: %v", agents)
	}
}

func TestSearchScrollback(t *testing.T) {
	_, ts := newTestServer(t)
	resp, _ := http.Post(ts.URL+"/v1/terms", "application/json", strings.NewReader(`{"name":"s1","command":"cat","cols":80,"rows":24}`))
	if resp.StatusCode != 201 {
		t.Fatalf("create: %v", resp.Status)
	}
	resp, _ = http.Post(ts.URL+"/v1/terms/s1/send", "application/json", strings.NewReader(`{"text":"needle-in-scrollback"}`))
	if resp.StatusCode != 204 {
		t.Fatalf("send: %v", resp.Status)
	}
	var hits []searchHit
	for i := 0; i < 50; i++ {
		resp, _ = http.Get(ts.URL + "/v1/search?q=NEEDLE")
		hits = nil
		_ = json.NewDecoder(resp.Body).Decode(&hits)
		if len(hits) == 1 && hits[0].Count >= 2 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if len(hits) != 1 || hits[0].Term != "s1" || hits[0].Count < 2 || len(hits[0].Lines) == 0 {
		t.Fatalf("hits: %+v", hits)
	}
	resp, _ = http.Get(ts.URL + "/v1/search?q=x")
	if resp.StatusCode != 400 {
		t.Fatalf("short query: %v", resp.Status)
	}
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/terms/s1", nil)
	_, _ = http.DefaultClient.Do(req)
}
