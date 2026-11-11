package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var claudeEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "Notification", "Stop", "StopFailure", "SubagentStart", "SubagentStop", "SessionEnd"}
var codexEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PermissionRequest", "Notification", "SubagentStart", "SubagentStop", "Stop", "Interrupt"}

// runHooks wires `keepty hook <agent>` into the agent's hook config without
// disturbing hooks that are already there (Orca's, yours). Idempotent.
func runHooks(args []string) error {
	if len(args) != 2 || (args[0] != "install" && args[0] != "show") {
		return errors.New("usage: keepty hooks install|show claude|codex")
	}
	agent := args[1]
	var path string
	var events []string
	home, _ := os.UserHomeDir()
	switch agent {
	case "claude":
		path = filepath.Join(home, ".claude", "settings.json")
		if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
			path = filepath.Join(d, "settings.json")
		}
		events = claudeEvents
	case "codex":
		path = filepath.Join(home, ".codex", "hooks.json")
		if d := os.Getenv("CODEX_HOME"); d != "" {
			path = filepath.Join(d, "hooks.json")
		}
		events = codexEvents
	default:
		return fmt.Errorf("unknown agent %q (claude|codex)", agent)
	}
	bin, _ := os.Executable()
	bin, _ = filepath.EvalSymlinks(bin)
	command := bin + " hook " + agent

	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	doc := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	added := mergeHooks(doc, events, command)
	out, _ := json.MarshalIndent(doc, "", "  ")
	if args[0] == "show" {
		fmt.Printf("would write %s (%d event(s) to add)\n", path, added)
		fmt.Println(string(out))
		return nil
	}
	if added == 0 {
		fmt.Printf("%s already has keepty hooks for every event\n", path)
		return nil
	}
	if len(raw) > 0 {
		_ = os.WriteFile(path+".bak-keepty", raw, 0o600)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %s (+%d event(s); backup at %s.bak-keepty)\n", path, added, path)
	if agent == "codex" {
		fmt.Println("Codex asks you to trust new hooks the first time it starts; answer yes once.")
	}
	return nil
}

// mergeHooks appends a {type: command, command: <command>} hook group to each
// event that does not already run a keepty hook. Returns how many were added.
func mergeHooks(doc map[string]any, events []string, command string) int {
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		doc["hooks"] = hooks
	}
	added := 0
	for _, ev := range events {
		groups, _ := hooks[ev].([]any)
		if hasKeeptyHook(groups) {
			continue
		}
		groups = append(groups, map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command, "timeout": 3}},
		})
		hooks[ev] = groups
		added++
	}
	return added
}

func hasKeeptyHook(groups []any) bool {
	for _, g := range groups {
		gm, _ := g.(map[string]any)
		inner, _ := gm["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, "keepty hook ") || strings.HasSuffix(cmd, "/keepty hook claude") || strings.HasSuffix(cmd, "/keepty hook codex") {
				return true
			}
		}
	}
	return false
}
