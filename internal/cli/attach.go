package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

const detachKey = 0x1c // Ctrl-\

func runAttach(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: fleet attach <name>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	return attach(c, args[0])
}

// attach mirrors the terminal: raw stdin to the PTY, PTY bytes to stdout,
// window size changes forwarded, Ctrl-\ detaches without ending the session.
func attach(c *client, name string) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("attach needs a terminal on stdin")
	}
	cols, rows := termSize()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := c.wsURL(fmt.Sprintf("/v1/terms/%s/ws?cols=%d&rows=%d", url.PathEscape(name), cols, rows))
	conn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPHeader: c.header()})
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return fmt.Errorf("no terminal named %q (see: fleet ls)", name)
		}
		if resp != nil && resp.StatusCode == 401 {
			return errors.New("unauthorized: set FLEET_TOKEN to the daemon's token")
		}
		return fmt.Errorf("connect %s: %w", u, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(8 << 20)

	old, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	restore := func() { _ = term.Restore(fd, old) }
	defer restore()
	fmt.Fprintf(os.Stdout, "\x1b[2m[fleet] attached to %s, detach with Ctrl-\\\x1b[0m\r\n", name)

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			w, h := termSize()
			msg, _ := json.Marshal(map[string]any{"t": "resize", "cols": w, "rows": h})
			if conn.Write(ctx, websocket.MessageText, msg) != nil {
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if i := bytes.IndexByte(buf[:n], detachKey); i >= 0 {
					if i > 0 {
						_ = conn.Write(ctx, websocket.MessageBinary, append([]byte(nil), buf[:i]...))
					}
					cancel()
					return
				}
				if conn.Write(ctx, websocket.MessageBinary, append([]byte(nil), buf[:n]...)) != nil {
					cancel()
					return
				}
			}
			if err != nil {
				cancel()
				return
			}
		}
	}()

	exitNote := ""
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		if typ == websocket.MessageBinary {
			_, _ = os.Stdout.Write(data)
			continue
		}
		var m struct {
			T    string `json:"t"`
			Code int    `json:"code"`
		}
		if json.Unmarshal(data, &m) == nil && m.T == "exit" {
			exitNote = fmt.Sprintf("terminal exited (%d)", m.Code)
			break
		}
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
	restore()
	if exitNote == "" {
		exitNote = "detached, session keeps running"
	}
	fmt.Fprintf(os.Stdout, "\r\n\x1b[2m[fleet] %s\x1b[0m\r\n", strings.TrimSpace(exitNote))
	return nil
}
