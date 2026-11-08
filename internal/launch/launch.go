// Package launch ties workspaces, agents and terminals together: make or find
// a workspace, open a terminal in it running an agent or a plain shell, switch
// an agent's model, or hand work from one agent to another.
package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/YoonShinWoong/fleet/internal/config"
	"github.com/YoonShinWoong/fleet/internal/terminal"
	"github.com/YoonShinWoong/fleet/internal/workspace"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type Launcher struct {
	cfg   *config.Config
	terms *terminal.Manager
	ws    *workspace.Store
}

func New(cfg *config.Config, terms *terminal.Manager, ws *workspace.Store) *Launcher {
	return &Launcher{cfg: cfg, terms: terms, ws: ws}
}

type AgentInfo struct {
	Name         string   `json:"name"`
	Command      string   `json:"command"`
	ModelFlag    string   `json:"modelFlag,omitempty"`
	Models       []string `json:"models,omitempty"`
	ModelCommand string   `json:"modelCommand,omitempty"`
}

// Agents lists what can run in a terminal: "shell" plus every configured agent.
func (l *Launcher) Agents() []AgentInfo {
	names := make([]string, 0, len(l.cfg.Agents))
	for k := range l.cfg.Agents {
		names = append(names, k)
	}
	sort.Strings(names)
	out := []AgentInfo{{Name: "shell"}}
	for _, n := range names {
		a := l.cfg.Agents[n]
		out = append(out, AgentInfo{Name: n, Command: a.Command, ModelFlag: a.ModelFlag, Models: a.Models, ModelCommand: a.ModelCommand})
	}
	return out
}

func (l *Launcher) Repos() []map[string]string {
	names := make([]string, 0, len(l.cfg.Repos))
	for k := range l.cfg.Repos {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]map[string]string, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]string{"name": n, "path": config.Expand(l.cfg.Repos[n])})
	}
	return out
}

type WorkspaceRequest struct {
	Name   string `json:"name"`
	Repo   string `json:"repo,omitempty"`   // configured repo name or checkout path: makes a git worktree
	Path   string `json:"path,omitempty"`   // any existing folder: no git involved
	Branch string `json:"branch,omitempty"` // worktree branch, default BranchPrefix+Name
}

// EnsureWorkspace returns the named workspace, creating it when needed.
func (l *Launcher) EnsureWorkspace(req WorkspaceRequest) (workspace.Workspace, bool, error) {
	if !nameRe.MatchString(req.Name) {
		return workspace.Workspace{}, false, fmt.Errorf("workspace name %q must match %s", req.Name, nameRe)
	}
	if w, ok := l.ws.Get(req.Name); ok {
		return w, false, nil
	}
	w := workspace.Workspace{Name: req.Name, CreatedAt: time.Now()}
	switch {
	case req.Path != "":
		p, err := filepath.Abs(config.Expand(req.Path))
		if err != nil {
			return w, false, err
		}
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			return w, false, fmt.Errorf("%s is not a directory", p)
		}
		w.Kind, w.Path = "folder", p
	case req.Repo != "":
		repoPath, repoName, err := resolveRepo(l.cfg, req.Repo)
		if err != nil {
			return w, false, err
		}
		branch := req.Branch
		if branch == "" {
			branch = l.cfg.BranchPrefix + req.Name
		}
		wt := filepath.Join(config.Expand(l.cfg.WorktreeRoot), repoName, req.Name)
		if _, err := os.Stat(wt); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
				return w, false, err
			}
			out, err := git(repoPath, "worktree", "add", "-b", branch, wt)
			if err != nil && strings.Contains(out, "already exists") {
				out, err = git(repoPath, "worktree", "add", wt, branch)
			}
			if err != nil {
				return w, false, fmt.Errorf("git worktree add: %s", out)
			}
		}
		w.Kind, w.Repo, w.RepoPath, w.Path, w.Branch = "worktree", repoName, repoPath, wt, branch
	default:
		return w, false, errors.New("workspace needs a repo or a path")
	}
	if err := l.ws.Add(w); err != nil {
		return w, false, err
	}
	return w, true, nil
}

// RemoveWorkspace forgets a workspace; with deleteFiles it also removes the git worktree.
func (l *Launcher) RemoveWorkspace(name string, deleteFiles bool) error {
	w, ok := l.ws.Get(name)
	if !ok {
		return workspace.ErrNotFound
	}
	for _, t := range l.terms.List() {
		if t.Workspace == name && t.Alive {
			return fmt.Errorf("workspace %q still has a running terminal (%s); kill it first", name, t.Name)
		}
	}
	if deleteFiles && w.Kind == "worktree" && w.RepoPath != "" {
		if out, err := git(w.RepoPath, "worktree", "remove", "--force", w.Path); err != nil {
			return fmt.Errorf("git worktree remove: %s", out)
		}
	}
	_, err := l.ws.Remove(name)
	return err
}

type OpenRequest struct {
	Workspace string `json:"workspace"`
	Agent     string `json:"agent"`            // "shell" (default) or a configured agent
	Name      string `json:"name,omitempty"`   // default <workspace>-<agent>
	Model     string `json:"model,omitempty"`  // passed with the agent's modelFlag
	Prompt    string `json:"prompt,omitempty"` // first prompt, passed as the agent's positional argument
	Cols      uint16 `json:"cols,omitempty"`
	Rows      uint16 `json:"rows,omitempty"`
}

// target is where a terminal runs: a workspace, or a bare folder for handoffs
// from terminals that were started outside any workspace.
type target struct {
	ws, path, repo string
}

// Open starts a terminal in a workspace.
func (l *Launcher) Open(req OpenRequest) (terminal.Info, error) {
	w, ok := l.ws.Get(req.Workspace)
	if !ok {
		return terminal.Info{}, fmt.Errorf("%w: %q", workspace.ErrNotFound, req.Workspace)
	}
	return l.openIn(target{ws: w.Name, path: w.Path, repo: w.Repo}, req)
}

func (l *Launcher) openIn(tg target, req OpenRequest) (terminal.Info, error) {
	agent := req.Agent
	if agent == "" {
		agent = "shell"
	}
	command, keepShell, label := "", false, ""
	if agent == "shell" {
		if req.Model != "" || req.Prompt != "" {
			return terminal.Info{}, errors.New("a shell takes no model or prompt")
		}
	} else {
		a, ok := l.cfg.Agents[agent]
		if !ok {
			names := []string{"shell"}
			for k := range l.cfg.Agents {
				names = append(names, k)
			}
			sort.Strings(names)
			return terminal.Info{}, fmt.Errorf("unknown agent %q (available: %s)", agent, strings.Join(names, ", "))
		}
		command = a.Command
		if req.Model != "" {
			if a.ModelFlag == "" {
				return terminal.Info{}, fmt.Errorf("agent %q has no modelFlag in config", agent)
			}
			command += " " + a.ModelFlag + " " + shellQuote(req.Model)
		}
		if req.Prompt != "" {
			command += " " + shellQuote(req.Prompt)
		}
		keepShell, label = true, agent
	}
	base := tg.ws
	if base == "" {
		base = filepath.Base(tg.path)
	}
	name := req.Name
	if name == "" {
		name = l.uniqueName(base + "-" + agent)
	} else if !nameRe.MatchString(name) {
		return terminal.Info{}, fmt.Errorf("terminal name %q must match %s", name, nameRe)
	}
	t, err := l.terms.Create(terminal.Options{
		Name: name, Command: command, Cwd: tg.path, Shell: l.cfg.Shell, KeepShell: keepShell,
		Size:  terminal.Size{Cols: req.Cols, Rows: req.Rows},
		Env:   map[string]string{"FLEET_WORKSPACE": tg.ws, "FLEET_TASK": base, "FLEET_REPO": tg.repo, "FLEET_AGENT": agent, "FLEET_MODEL": req.Model},
		Agent: label, Task: base, Repo: tg.repo, Workspace: tg.ws, Model: req.Model,
	})
	if err != nil {
		return terminal.Info{}, err
	}
	return t.Info(), nil
}

// SetModel types the agent's model command into a running terminal.
func (l *Launcher) SetModel(term, model string) error {
	t, ok := l.terms.Get(term)
	if !ok {
		return terminal.ErrNotFound
	}
	info := t.Info()
	if !info.Alive {
		return fmt.Errorf("terminal %q has exited", term)
	}
	if info.Agent == "" {
		return fmt.Errorf("terminal %q runs a plain shell; models apply to agents", term)
	}
	a := l.cfg.Agents[info.Agent]
	if a.ModelCommand == "" {
		return fmt.Errorf("agent %q has no modelCommand in config", info.Agent)
	}
	line := strings.ReplaceAll(a.ModelCommand, "{model}", model)
	if err := t.Write([]byte(line + "\r")); err != nil {
		return err
	}
	t.SetModel(model)
	return nil
}

type HandoffRequest struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Model string `json:"model,omitempty"`
	Name  string `json:"name,omitempty"`
	Close bool   `json:"close,omitempty"` // kill the source terminal once the new one is up
	Cols  uint16 `json:"cols,omitempty"`
	Rows  uint16 `json:"rows,omitempty"`
}

type HandoffResult struct {
	Term   terminal.Info `json:"term"`
	File   string        `json:"file"`
	Closed bool          `json:"closed"`
}

// Handoff writes what the source terminal was doing into <dir>/.fleet/handoff-*.md
// and starts the target agent in the same place with a prompt pointing at it.
func (l *Launcher) Handoff(req HandoffRequest) (HandoffResult, error) {
	from, ok := l.terms.Get(req.From)
	if !ok {
		return HandoffResult{}, terminal.ErrNotFound
	}
	if req.To == "" || req.To == "shell" {
		return HandoffResult{}, errors.New("handoff target must be an agent")
	}
	info := from.Info()
	tg := target{path: info.Cwd}
	if info.Workspace != "" {
		if w, ok := l.ws.Get(info.Workspace); ok {
			tg = target{ws: w.Name, path: w.Path, repo: w.Repo}
		}
	}
	file, err := writeHandoff(tg.path, info, req.To, from.Scrollback())
	if err != nil {
		return HandoffResult{}, err
	}
	rel, err := filepath.Rel(tg.path, file)
	if err != nil {
		rel = file
	}
	prompt := strings.NewReplacer("{from}", agentLabel(info), "{to}", req.To, "{file}", rel).Replace(l.cfg.HandoffPrompt)
	nt, err := l.openIn(tg, OpenRequest{Agent: req.To, Model: req.Model, Prompt: prompt, Name: req.Name, Cols: req.Cols, Rows: req.Rows})
	if err != nil {
		return HandoffResult{File: file}, err
	}
	res := HandoffResult{Term: nt, File: file}
	if req.Close {
		if err := l.terms.Kill(req.From); err == nil {
			res.Closed = true
		}
	}
	return res, nil
}

func agentLabel(info terminal.Info) string {
	if info.Agent != "" {
		return info.Agent
	}
	return "shell"
}

func writeHandoff(dir string, info terminal.Info, to string, scrollback []byte) (string, error) {
	fdir := filepath.Join(dir, ".fleet")
	if err := os.MkdirAll(fdir, 0o755); err != nil {
		return "", err
	}
	excludeFromGit(dir)
	var b strings.Builder
	fmt.Fprintf(&b, "# Handoff: %s → %s\n\n", agentLabel(info), to)
	fmt.Fprintf(&b, "- 시각: %s\n- 이전 터미널: %s (%s)\n- 작업 폴더: %s\n", time.Now().Format(time.RFC3339), info.Name, info.Command, dir)
	if info.Workspace != "" {
		fmt.Fprintf(&b, "- 워크스페이스: %s\n", info.Workspace)
	}
	if branch := gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		fmt.Fprintf(&b, "- 브랜치: %s\n", branch)
	}
	b.WriteString("\n## 변경 상태 (git)\n\n")
	status := gitOut(dir, "status", "--short")
	if status == "" {
		status = "(변경 없음, 또는 git 저장소가 아님)"
	}
	fmt.Fprintf(&b, "```\n%s\n```\n\n", tail(status, 60))
	if stat := gitOut(dir, "diff", "--stat"); stat != "" {
		fmt.Fprintf(&b, "```\n%s\n```\n\n", tail(stat, 40))
	}
	b.WriteString("## 이전 터미널의 마지막 출력\n\n이전 에이전트가 마지막으로 보고 있던 화면이다. 무엇을 하고 있었고 무엇이 남았는지 여기서 파악한 뒤 이어서 진행하라.\n\n```text\n")
	b.WriteString(tail(string(terminal.PlainText(scrollback)), 200))
	b.WriteString("\n```\n")
	path := filepath.Join(fdir, fmt.Sprintf("handoff-%s-%s-to-%s.md", time.Now().Format("20060102-150405"), info.Name, to))
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

// excludeFromGit keeps .fleet/ out of git status without touching .gitignore.
func excludeFromGit(dir string) {
	p, err := git(dir, "rev-parse", "--git-path", "info/exclude")
	if err != nil || p == "" {
		return
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	cur, _ := os.ReadFile(p)
	if strings.Contains(string(cur), ".fleet/") {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(cur) > 0 && cur[len(cur)-1] != '\n' {
		_, _ = f.WriteString("\n")
	}
	_, _ = f.WriteString(".fleet/\n")
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (l *Launcher) uniqueName(base string) string {
	name := base
	for i := 2; ; i++ {
		t, ok := l.terms.Get(name)
		if !ok || !t.Info().Alive {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// Request is the one-shot form: repo + task → workspace named task + an agent in it.
type Request struct {
	Repo  string `json:"repo"`
	Task  string `json:"task"`
	Agent string `json:"agent"`
	Model string `json:"model,omitempty"`
	Cols  uint16 `json:"cols,omitempty"`
	Rows  uint16 `json:"rows,omitempty"`
}

type Result struct {
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Worktree  string `json:"worktree"`
	Branch    string `json:"branch"`
	Term      string `json:"term,omitempty"`
	Command   string `json:"command"`
}

func (l *Launcher) NewTask(req Request) (Result, error) {
	w, _, err := l.EnsureWorkspace(WorkspaceRequest{Name: req.Task, Repo: req.Repo})
	if err != nil {
		return Result{}, err
	}
	res := Result{Workspace: w.Name, Repo: w.Repo, Worktree: w.Path, Branch: w.Branch}
	if req.Agent == "" {
		req.Agent = "claude"
	}
	info, err := l.Open(OpenRequest{Workspace: w.Name, Agent: req.Agent, Model: req.Model, Cols: req.Cols, Rows: req.Rows})
	if err != nil {
		return res, err
	}
	res.Term, res.Command = info.Name, info.Command
	return res, nil
}

// resolveRepo accepts a configured repo name or a path to a git checkout.
func resolveRepo(cfg *config.Config, repo string) (string, string, error) {
	if p, ok := cfg.Repos[repo]; ok {
		return config.Expand(p), repo, nil
	}
	p := config.Expand(repo)
	if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
		abs, _ := filepath.Abs(p)
		return abs, filepath.Base(abs), nil
	}
	return "", "", fmt.Errorf("repo %q is neither a configured name nor a git checkout path", repo)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitOut(dir string, args ...string) string {
	out, err := git(dir, args...)
	if err != nil {
		return ""
	}
	return out
}
