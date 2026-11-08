package cli

import (
	"errors"
	"flag"
	"fmt"

	"github.com/YoonShinWoong/fleet/internal/launch"
)

func runNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	agent := fs.String("agent", "claude", "agent to launch (claude|codex|... as configured)")
	model := fs.String("model", "", "model to start the agent with")
	doAttach := fs.Bool("a", false, "attach right away")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return errors.New("usage: fleet new <repo> <task> [-agent claude|codex] [-a]")
	}
	repo, task := pos[0], pos[1]
	c, err := newClient()
	if err != nil {
		return err
	}
	cols, rows := termSize()
	var res launch.Result
	err = c.do("POST", "/v1/tasks", launch.Request{Repo: repo, Task: task, Agent: *agent, Model: *model, Cols: cols, Rows: rows}, &res)
	if res.Worktree != "" {
		fmt.Printf("workspace %s\nworktree  %s\nbranch    %s\n", res.Workspace, res.Worktree, res.Branch)
	}
	if err != nil {
		return err
	}
	fmt.Printf("terminal  %s  (%s: %s)\n", res.Term, *agent, res.Command)
	if *doAttach {
		return attach(c, res.Term)
	}
	fmt.Printf("\nattach with: fleet attach %s\n", res.Term)
	return nil
}
