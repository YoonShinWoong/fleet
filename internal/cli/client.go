package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/YoonShinWoong/keepty/internal/config"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

type client struct {
	cfg   *config.Config
	base  string
	token string
}

func newClient() (*client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &client{cfg: cfg, base: config.ClientBase(cfg), token: config.ClientToken(cfg)}, nil
}

func (c *client) wsURL(path string) string {
	u := c.base + path
	u = strings.Replace(u, "https://", "wss://", 1)
	return strings.Replace(u, "http://", "ws://", 1)
}

func (c *client) header() http.Header {
	h := http.Header{}
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	return h
}

func (c *client) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header = c.header()
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("daemon unreachable at %s (%v). Is `keepty daemon` running?", c.base, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		return fmt.Errorf("%s: %s", resp.Status, e.Error)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
