package cli

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"text/tabwriter"

	"github.com/YoonShinWoong/keepty/internal/workspace"
)

func runWS(args []string) error {
	if len(args) == 0 {
		args = []string{"ls"}
	}
	switch args[0] {
	case "ls":
		return wsList()
	case "new", "add":
		return wsNew(args[1:])
	case "rm", "remove":
		return wsRemove(args[1:])
	}
	return errors.New("usage: keepty ws ls | new <name> (-repo r | -path p) [-branch b] | rm <name> [-delete]")
}

func wsList() error {
	c, err := newClient()
	if err != nil {
		return err
	}
	var list []workspace.Workspace
	if err := c.do("GET", "/v1/workspaces", nil, &list); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tKIND\tREPO\tBRANCH\tPATH\tAGE")
	for _, w := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", w.Name, w.Kind, w.Repo, w.Branch, w.Path, humanAge(w.CreatedAt))
	}
	if len(list) == 0 {
		fmt.Fprintln(tw, "(no workspaces)\t\t\t\t\t")
	}
	return tw.Flush()
}

func wsNew(args []string) error {
	fs := flag.NewFlagSet("ws new", flag.ContinueOnError)
	repo := fs.String("repo", "", "configured repo name or checkout path (makes a git worktree)")
	path := fs.String("path", "", "existing folder to use as is")
	branch := fs.String("branch", "", "worktree branch (default: branchPrefix + name)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || (*repo == "" && *path == "") {
		return errors.New("usage: keepty ws new <name> (-repo r | -path p) [-branch b]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	var w workspace.Workspace
	if err := c.do("POST", "/v1/workspaces", map[string]string{"name": pos[0], "repo": *repo, "path": *path, "branch": *branch}, &w); err != nil {
		return err
	}
	fmt.Printf("workspace %s  (%s)\npath      %s\n", w.Name, w.Kind, w.Path)
	if w.Branch != "" {
		fmt.Printf("branch    %s\n", w.Branch)
	}
	fmt.Printf("\nopen with: keepty open %s -agent claude|codex|shell\n", w.Name)
	return nil
}

func wsRemove(args []string) error {
	fs := flag.NewFlagSet("ws rm", flag.ContinueOnError)
	del := fs.Bool("delete", false, "also remove the git worktree directory")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: keepty ws rm <name> [-delete]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	q := ""
	if *del {
		q = "?delete=1"
	}
	return c.do("DELETE", "/v1/workspaces/"+url.PathEscape(pos[0])+q, nil, nil)
}
