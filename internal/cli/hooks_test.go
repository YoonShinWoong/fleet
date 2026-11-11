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
