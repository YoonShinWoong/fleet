package launch

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YoonShinWoong/keepty/internal/config"
	"github.com/YoonShinWoong/keepty/internal/terminal"
	"github.com/YoonShinWoong/keepty/internal/workspace"
)

func TestWorktreeWorkspaceAndShell(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", repo},
		{"-C", repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	cfg := config.Default()
	cfg.Shell = "/bin/sh"
	cfg.WorktreeRoot = filepath.Join(dir, "wt")
	cfg.Repos = map[string]string{"repo": repo}
	ws, _ := workspace.Open(filepath.Join(dir, "ws.json"))
	l := New(cfg, terminal.NewManager(4096, nil), ws)

	w, created, err := l.EnsureWorkspace(WorkspaceRequest{Name: "fix-login", Repo: "repo"})
	if err != nil || !created || w.Kind != "worktree" || w.Branch != "fix-login" {
		t.Fatalf("ensure: %+v %v %v", w, created, err)
	}
	if _, again, _ := l.EnsureWorkspace(WorkspaceRequest{Name: "fix-login", Repo: "repo"}); again {
		t.Fatal("second ensure should reuse")
	}
	info, err := l.Open(OpenRequest{Workspace: "fix-login", Agent: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "fix-login-shell" || info.Workspace != "fix-login" || info.Cwd != w.Path || info.Agent != "" {
		t.Fatalf("info: %+v", info)
	}
	info2, _ := l.Open(OpenRequest{Workspace: "fix-login", Agent: "shell"})
	if info2.Name != "fix-login-shell-2" {
		t.Fatalf("dedupe: %s", info2.Name)
	}
	if err := l.RemoveWorkspace("fix-login", false); err == nil {
		t.Fatal("remove should refuse while terminals run")
	}
	_ = l.terms.Kill(info.Name)
	_ = l.terms.Kill(info2.Name)
	if err := l.RemoveWorkspace("fix-login", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Get("fix-login"); ok {
		t.Fatal("still listed")
	}
}

func TestFolderWorkspaceAndUnknownAgent(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	ws, _ := workspace.Open(filepath.Join(dir, "ws.json"))
	l := New(cfg, terminal.NewManager(4096, nil), ws)
	w, _, err := l.EnsureWorkspace(WorkspaceRequest{Name: "notes", Path: dir})
	if err != nil || w.Kind != "folder" || w.Path != dir {
		t.Fatalf("%+v %v", w, err)
	}
	if _, err := l.Open(OpenRequest{Workspace: "notes", Agent: "nope"}); err == nil {
		t.Fatal("unknown agent should fail")
	}
	if _, _, err := l.EnsureWorkspace(WorkspaceRequest{Name: "bad name"}); err == nil {
		t.Fatal("bad name should fail")
	}
}

func TestModelFlagSetModelAndHandoff(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Shell = "/bin/sh"
	// "printer" echoes its argv so we can see the flag, model and prompt it was given.
	cfg.Agents["printer"] = config.Agent{Command: "printf '%s\\n'", ModelFlag: "--model", ModelCommand: "/model {model}"}
	ws, _ := workspace.Open(filepath.Join(dir, "ws.json"))
	l := New(cfg, terminal.NewManager(1<<16, nil), ws)
	if _, _, err := l.EnsureWorkspace(WorkspaceRequest{Name: "w", Path: dir}); err != nil {
		t.Fatal(err)
	}

	// source: a cat terminal with some context typed in
	src, err := l.Open(OpenRequest{Workspace: "w", Agent: "shell", Name: "src"})
	if err != nil {
		t.Fatal(err)
	}
	srcTerm, _ := l.terms.Get("src")
	_ = srcTerm.Write([]byte("cat\nhello-handoff-context\n"))
	waitFor(t, func() bool { return bytes.Contains(srcTerm.Scrollback(), []byte("hello-handoff-context")) })

	// SetModel on a shell must fail; on an agent it types the command
	if err := l.SetModel("src", "opus"); err == nil {
		t.Fatal("SetModel on a shell should fail")
	}

	res, err := l.Handoff(HandoffRequest{From: "src", To: "printer", Model: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Term.Workspace != "w" || res.Term.Agent != "printer" || res.Term.Model != "opus" || !strings.HasPrefix(res.Term.Name, "w-printer") {
		t.Fatalf("term: %+v", res.Term)
	}
	body, err := os.ReadFile(res.File)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Handoff: shell → printer", "hello-handoff-context", "src"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("handoff file lacks %q:\n%s", want, body)
		}
	}
	nt, _ := l.terms.Get(res.Term.Name)
	waitFor(t, func() bool {
		sb := nt.Scrollback()
		return bytes.Contains(sb, []byte("--model")) && bytes.Contains(sb, []byte("opus")) && bytes.Contains(sb, []byte(filepath.Base(res.File)))
	})
	if src.Name != "src" {
		t.Fatal("unexpected")
	}
	_ = l.terms.Kill("src")
	_ = l.terms.Kill(res.Term.Name)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
