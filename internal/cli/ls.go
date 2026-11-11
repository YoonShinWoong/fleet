package cli

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/YoonShinWoong/keepty/internal/state"
	"github.com/YoonShinWoong/keepty/internal/terminal"
)

func runLs(args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	all := fs.Bool("a", false, "include ended sessions and exited terminals")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	var terms []terminal.Info
	var sessions []state.Session
	if err := c.do("GET", "/v1/terms", nil, &terms); err != nil {
		return err
	}
	if err := c.do("GET", "/v1/sessions", nil, &sessions); err != nil {
		return err
	}
	byTerm := map[string]state.Session{}
	for _, s := range sessions {
		if s.Term == "" {
			continue
		}
		if cur, ok := byTerm[s.Term]; !ok || s.UpdatedAt.After(cur.UpdatedAt) {
			byTerm[s.Term] = s
		}
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tWORKSPACE\tAGENT\tSTATUS\tLAST\tCLIENTS\tSIZE\tAGE")
	n := 0
	for _, t := range terms {
		if !t.Alive && !*all {
			continue
		}
		agent, status, last, model := t.Agent, "shell", "", t.Model
		if !t.Alive {
			status = fmt.Sprintf("exited(%d)", t.ExitCode)
		}
		if s, ok := byTerm[t.Name]; ok {
			agent, status = s.Agent, string(s.Status)
			if s.Model != "" {
				model = s.Model
			}
			last = s.LastTool
			if s.Status != state.StatusWorking && s.LastMessage != "" {
				last = s.LastMessage
			}
			if s.Subagents > 0 {
				last = fmt.Sprintf("%s (+%d sub)", last, s.Subagents)
			}
		}
		if model != "" {
			agent = agent + "/" + model
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%dx%d\t%s\n", clip(t.Name, 28), clip(t.Workspace, 20), clip(agent, 22), status, clip(last, 48), t.Clients, t.Size.Cols, t.Size.Rows, humanAge(t.StartedAt))
		n++
	}
	for _, s := range sessions {
		if s.Term != "" || (s.Status == state.StatusEnded && !*all) {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t-\t-\t%s\n", clip("("+s.Label()+")", 28), clip(s.Task, 20), s.Agent, s.Status, clip(s.LastMessage, 48), humanAge(s.UpdatedAt))
		n++
	}
	if n == 0 {
		fmt.Fprintln(tw, "(nothing running)\t\t\t\t\t\t\t")
	}
	return tw.Flush()
}
