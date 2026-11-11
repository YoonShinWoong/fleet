package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/YoonShinWoong/keepty/internal/config"
	"github.com/YoonShinWoong/keepty/internal/hooks"
)

// runHook never fails: a broken control plane must never block an agent.
// It always prints {} so Claude Code and Codex treat the hook as "no decision".
func runHook(args []string) error {
	defer fmt.Println("{}")
	agent := "claude"
	if len(args) > 0 {
		agent = args[0]
	}
	payload, _ := io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
	if len(bytes.TrimSpace(payload)) == 0 {
		payload = []byte("{}")
	}
	if !json.Valid(payload) {
		debugf("stdin is not json (%d bytes)", len(payload))
		return nil
	}
	host, _ := os.Hostname()
	env := hooks.Envelope{
		Context: hooks.Context{
			Agent:      agent,
			Host:       host,
			Term:       os.Getenv("KEEPTY_TERM"),
			Task:       os.Getenv("KEEPTY_TASK"),
			Repo:       os.Getenv("KEEPTY_REPO"),
			ReceivedAt: time.Now(),
		},
		Payload: payload,
	}
	cfg, err := config.Load()
	if err != nil {
		debugf("config: %v", err)
		return nil
	}
	body, _ := json.Marshal(env)
	req, _ := http.NewRequest(http.MethodPost, config.ClientBase(cfg)+"/v1/events/"+agent, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if tok := config.ClientToken(cfg); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := (&http.Client{Timeout: 1500 * time.Millisecond}).Do(req)
	if err != nil {
		debugf("post: %v", err)
		return nil
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		debugf("daemon replied %s", resp.Status)
	}
	return nil
}

func debugf(format string, a ...any) {
	if os.Getenv("KEEPTY_HOOK_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "keepty hook: "+format+"\n", a...)
	}
}
