package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Agent describes how to launch one coding agent inside a fleet terminal.
type Agent struct {
	Command      string   `json:"command"`
	ModelFlag    string   `json:"modelFlag,omitempty"`    // appended as "<flag> <model>" at launch, e.g. --model or -m
	Models       []string `json:"models,omitempty"`       // suggestions for pickers; any string is accepted
	ModelCommand string   `json:"modelCommand,omitempty"` // typed into a running session; {model} is substituted
	// ResumeCommand restarts the agent on a session after the daemon restarts; {session} is the
	// session id the agent reported through its hooks.
	ResumeCommand string `json:"resumeCommand,omitempty"`
}

// Config is ~/.fleet/config.json. Missing keys fall back to Default().
type Config struct {
	Addr            string            `json:"addr"`
	Token           string            `json:"token"`
	Shell           string            `json:"shell"`
	ScrollbackBytes int               `json:"scrollbackBytes"`
	WorktreeRoot    string            `json:"worktreeRoot"`
	BranchPrefix    string            `json:"branchPrefix"`
	Repos           map[string]string `json:"repos"`
	Agents          map[string]Agent  `json:"agents"`
	HandoffPrompt   string            `json:"handoffPrompt"` // {from} {to} {file} are substituted
}

func Default() *Config {
	return &Config{
		Addr:            "127.0.0.1:7677",
		ScrollbackBytes: 1 << 20,
		WorktreeRoot:    "~/fleet/worktrees",
		Repos:           map[string]string{},
		Agents: map[string]Agent{
			"claude": {Command: "claude --remote-control", ModelFlag: "--model", Models: []string{"fable", "opus", "sonnet", "haiku"}, ModelCommand: "/model {model}", ResumeCommand: "claude --remote-control --resume {session}"},
			"codex":  {Command: "codex", ModelFlag: "-m", ModelCommand: "/model", ResumeCommand: "codex resume {session}"},
		},
		HandoffPrompt: "이전 에이전트({from})에서 이어받은 작업이야. 먼저 {file} 을 읽고 맥락과 남은 일을 파악한 뒤 이어서 진행해. 시작할 때 네가 이해한 현재 상태를 한 문단으로 요약해줘.",
	}
}

// backfill fills agent fields a user's older config.json never had, so model
// features work without editing the file. Only empty fields are touched.
func (c *Config) backfill() {
	def := Default()
	for name, a := range c.Agents {
		d, ok := def.Agents[name]
		if !ok {
			continue
		}
		if a.ModelFlag == "" {
			a.ModelFlag = d.ModelFlag
		}
		if len(a.Models) == 0 {
			a.Models = d.Models
		}
		if a.ModelCommand == "" {
			a.ModelCommand = d.ModelCommand
		}
		if a.ResumeCommand == "" {
			a.ResumeCommand = d.ResumeCommand
		}
		c.Agents[name] = a
	}
	if c.HandoffPrompt == "" {
		c.HandoffPrompt = def.HandoffPrompt
	}
}

// Dir is $FLEET_HOME or ~/.fleet.
func Dir() string {
	if d := os.Getenv("FLEET_HOME"); d != "" {
		return Expand(d)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleet")
}

func Path() string           { return filepath.Join(Dir(), "config.json") }
func StatePath() string      { return filepath.Join(Dir(), "state.json") }
func WorkspacesPath() string { return filepath.Join(Dir(), "workspaces.json") }
func TerminalsPath() string  { return filepath.Join(Dir(), "terminals.json") }

// Load reads config.json over the defaults. A missing file is not an error.
func Load() (*Config, error) {
	cfg := Default()
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	cfg.backfill()
	return cfg, nil
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(b, '\n'), 0o600)
}

// Expand resolves a leading ~ to the home directory.
func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

// ClientBase is the daemon URL a client talks to: $FLEET_ADDR wins over config.
func ClientBase(cfg *Config) string {
	addr := os.Getenv("FLEET_ADDR")
	if addr == "" {
		addr = cfg.Addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + addr
}

// ClientToken is $FLEET_TOKEN, else the local config token.
func ClientToken(cfg *Config) string {
	if t := os.Getenv("FLEET_TOKEN"); t != "" {
		return t
	}
	return cfg.Token
}
