// Package hooks turns Claude Code and Codex hook payloads into session state.
//
// Both CLIs send one JSON object per event on stdin. The field names they share
// (session_id, hook_event_name, cwd, tool_name, ...) are extracted; everything
// else is ignored so schema changes on either side never break ingestion.
package hooks

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/YoonShinWoong/fleet/internal/state"
)

// Context is what the `fleet hook` client adds around the raw payload.
type Context struct {
	Agent      string    `json:"agent"`
	Host       string    `json:"host,omitempty"`
	Term       string    `json:"term,omitempty"`
	Task       string    `json:"task,omitempty"`
	Repo       string    `json:"repo,omitempty"`
	ReceivedAt time.Time `json:"receivedAt"`
}

type Envelope struct {
	Context Context         `json:"context"`
	Payload json.RawMessage `json:"payload"`
}

// fields is the tolerant projection of a hook payload.
type fields struct {
	SessionID        string `json:"session_id"`
	Event            string `json:"hook_event_name"`
	Cwd              string `json:"cwd"`
	ToolName         string `json:"tool_name"`
	Message          string `json:"message"`
	Title            string `json:"title"`
	NotificationType string `json:"notification_type"`
	Model            string `json:"model"`
	Prompt           string `json:"prompt"`
	Reason           string `json:"reason"`
	LastAssistant    string `json:"last_assistant_message"`
}

// Apply folds one event into the store and returns the updated session.
func Apply(store *state.Store, env Envelope) (state.Session, error) {
	var f fields
	_ = json.Unmarshal(env.Payload, &f)
	if f.SessionID == "" {
		return state.Session{}, errors.New("payload has no session_id")
	}
	if env.Context.ReceivedAt.IsZero() {
		env.Context.ReceivedAt = time.Now()
	}
	return store.Update(f.SessionID, func(s *state.Session) { Reduce(s, env.Context, f) }), nil
}

// Reduce is the state machine. Exported for tests.
func Reduce(s *state.Session, c Context, f fields) {
	if s.StartedAt.IsZero() {
		s.StartedAt = c.ReceivedAt
	}
	s.UpdatedAt = c.ReceivedAt
	s.Events++
	if c.Agent != "" {
		s.Agent = c.Agent
	}
	if c.Host != "" {
		s.Host = c.Host
	}
	if c.Term != "" {
		s.Term = c.Term
	}
	if c.Task != "" {
		s.Task = c.Task
	}
	if c.Repo != "" {
		s.Repo = c.Repo
	}
	if f.Cwd != "" {
		s.Cwd = f.Cwd
	}
	if f.Model != "" {
		s.Model = f.Model
	}
	s.LastEvent = f.Event

	switch f.Event {
	case "SessionStart":
		s.Status = state.StatusIdle
		s.Subagents = 0
	case "UserPromptSubmit":
		s.Status = state.StatusWorking
		s.LastMessage = truncate(f.Prompt, 140)
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		s.Status = state.StatusWorking
		if f.ToolName != "" {
			s.LastTool = f.ToolName
		}
	case "PermissionRequest":
		s.Status = state.StatusNeedsInput
		s.LastMessage = "permission: " + f.ToolName
	case "Elicitation":
		s.Status = state.StatusNeedsInput
		s.LastMessage = firstNonEmpty(f.Message, f.Title, "elicitation")
	case "Notification":
		switch f.NotificationType {
		case "permission_prompt", "idle_prompt", "elicitation_dialog":
			s.Status = state.StatusNeedsInput
		}
		if m := firstNonEmpty(f.Message, f.Title); m != "" {
			s.LastMessage = truncate(m, 140)
		}
	case "Stop", "Interrupt":
		s.Status = state.StatusIdle
		if f.LastAssistant != "" {
			s.LastMessage = truncate(f.LastAssistant, 140)
		}
	case "StopFailure":
		s.Status = state.StatusError
		s.LastMessage = truncate(firstNonEmpty(f.Reason, f.Message, "stop failure"), 140)
	case "SubagentStart":
		s.Subagents++
	case "SubagentStop":
		if s.Subagents > 0 {
			s.Subagents--
		}
	case "SessionEnd":
		s.Status = state.StatusEnded
		s.Subagents = 0
	default:
		if s.Status == "" {
			s.Status = state.StatusUnknown
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
