package cli

import (
	"encoding/json"
	"testing"
)

func TestMergeHooksKeepsExistingAndIsIdempotent(t *testing.T) {
	var doc map[string]any
	_ = json.Unmarshal([]byte(`{"model":"fable","hooks":{"Stop":[{"hooks":[{"type":"command","command":"orca-hook.sh"}]}]}}`), &doc)
	n := mergeHooks(doc, []string{"Stop", "SessionStart"}, "/usr/local/bin/keepty hook claude")
	if n != 2 {
		t.Fatalf("added %d", n)
	}
	stop := doc["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 2 {
		t.Fatalf("Stop groups: %d (existing hook must be kept)", len(stop))
	}
	if doc["model"] != "fable" {
		t.Fatal("other settings must survive")
	}
	if again := mergeHooks(doc, []string{"Stop", "SessionStart"}, "/usr/local/bin/keepty hook claude"); again != 0 {
		t.Fatalf("second merge added %d", again)
	}
}

func TestRemoveLegacyHooksKeepsOthers(t *testing.T) {
	var doc map[string]any
	_ = json.Unmarshal([]byte(`{"hooks":{
		"Stop":[{"hooks":[{"type":"command","command":"fleet hook claude"}]},{"hooks":[{"type":"command","command":"orca-hook.sh"}]}],
		"SessionStart":[{"hooks":[{"type":"command","command":"/Users/me/.local/bin/fleet hook claude"},{"type":"command","command":"other.sh"}]}],
		"PreToolUse":[{"hooks":[{"type":"command","command":"fleet hook codex"}]}],
		"Notification":[{"hooks":[{"type":"command","command":"fleetctl hook claude"}]}]
	}}`), &doc)
	if n := removeLegacyHooks(doc); n != 3 {
		t.Fatalf("removed %d, want 3", n)
	}
	hooks := doc["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; ok {
		t.Fatal("an event left with no hooks must be dropped")
	}
	if stop := hooks["Stop"].([]any); len(stop) != 1 {
		t.Fatalf("Stop groups: %d, want only orca's", len(stop))
	}
	start := hooks["SessionStart"].([]any)[0].(map[string]any)["hooks"].([]any)
	if len(start) != 1 || start[0].(map[string]any)["command"] != "other.sh" {
		t.Fatalf("SessionStart kept %v", start)
	}
	if len(hooks["Notification"].([]any)) != 1 {
		t.Fatal("a command that only looks similar must be kept")
	}
	if again := removeLegacyHooks(doc); again != 0 {
		t.Fatalf("second pass removed %d", again)
	}
}
