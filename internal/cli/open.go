package cli

import (
	"errors"
	"flag"
	"fmt"

	"github.com/YoonShinWoong/keepty/internal/terminal"
)

// runOpen starts an agent or shell terminal inside an existing workspace.
func runOpen(args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	agent := fs.String("agent", "shell", "shell, claude, codex, or any configured agent")
	name := fs.String("name", "", "terminal name (default: <workspace>-<agent>)")
	model := fs.String("model", "", "model to start the agent with")
	prompt := fs.String("prompt", "", "first prompt for the agent")
	doAttach := fs.Bool("a", false, "attach right away")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: keepty open <workspace> [-agent shell|claude|codex] [-name n] [-a]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	cols, rows := termSize()
	var info terminal.Info
	body := map[string]any{"workspace": pos[0], "agent": *agent, "name": *name, "model": *model, "prompt": *prompt, "cols": cols, "rows": rows}
	if err := c.do("POST", "/v1/terms", body, &info); err != nil {
		return err
	}
	fmt.Printf("terminal  %s  in %s (%s)\n", info.Name, info.Workspace, info.Command)
	if *doAttach {
		return attach(c, info.Name)
	}
	fmt.Printf("attach with: keepty attach %s\n", info.Name)
	return nil
}
