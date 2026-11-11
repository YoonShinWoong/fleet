package hooks

import (
	"testing"
	"time"

	"github.com/YoonShinWoong/keepty/internal/state"
)

func TestReduceStatus(t *testing.T) {
	now := time.Now()
	cases := []struct {
		event, ntype string
		want         state.Status
	}{
		{"SessionStart", "", state.StatusIdle},
		{"UserPromptSubmit", "", state.StatusWorking},
		{"PreToolUse", "", state.StatusWorking},
		{"PermissionRequest", "", state.StatusNeedsInput},
		{"Notification", "idle_prompt", state.StatusNeedsInput},
		{"Stop", "", state.StatusIdle},
		{"Interrupt", "", state.StatusIdle},
		{"StopFailure", "", state.StatusError},
		{"SessionEnd", "", state.StatusEnded},
	}
	for _, c := range cases {
		s := &state.Session{ID: "x"}
		Reduce(s, Context{Agent: "claude", ReceivedAt: now}, fields{SessionID: "x", Event: c.event, NotificationType: c.ntype})
		if s.Status != c.want {
			t.Errorf("%s/%s: got %s want %s", c.event, c.ntype, s.Status, c.want)
		}
	}
}

func TestNotificationWithoutTypeKeepsStatus(t *testing.T) {
	s := &state.Session{ID: "x", Status: state.StatusWorking}
	Reduce(s, Context{ReceivedAt: time.Now()}, fields{SessionID: "x", Event: "Notification", Message: "auth ok"})
	if s.Status != state.StatusWorking {
		t.Fatalf("got %s", s.Status)
	}
	if s.LastMessage != "auth ok" {
		t.Fatalf("got %q", s.LastMessage)
	}
}

func TestSubagentCounter(t *testing.T) {
	s := &state.Session{ID: "x"}
	ctx := Context{ReceivedAt: time.Now()}
	Reduce(s, ctx, fields{SessionID: "x", Event: "SubagentStart"})
	Reduce(s, ctx, fields{SessionID: "x", Event: "SubagentStart"})
	Reduce(s, ctx, fields{SessionID: "x", Event: "SubagentStop"})
	if s.Subagents != 1 {
		t.Fatalf("got %d", s.Subagents)
	}
	Reduce(s, ctx, fields{SessionID: "x", Event: "SessionEnd"})
	if s.Subagents != 0 || s.Status != state.StatusEnded {
		t.Fatalf("got %d %s", s.Subagents, s.Status)
	}
}
