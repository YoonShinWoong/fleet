package cli

import (
	"errors"
	"flag"
	"fmt"
	"net/url"

	"github.com/YoonShinWoong/fleet/internal/launch"
)

func runModel(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: fleet model <terminal> <model>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	return c.do("POST", "/v1/terms/"+url.PathEscape(args[0])+"/model", map[string]string{"model": args[1]}, nil)
}

func runHandoff(args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	to := fs.String("to", "", "target agent (claude, codex, ...)")
	model := fs.String("model", "", "model for the target agent")
	name := fs.String("name", "", "name for the new terminal")
	closeSrc := fs.Bool("close", false, "kill the source terminal once the new one is up")
	doAttach := fs.Bool("a", false, "attach to the new terminal")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *to == "" {
		return errors.New("usage: fleet handoff <terminal> -to <agent> [-model m] [-name n] [-close] [-a]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	cols, rows := termSize()
	var res launch.HandoffResult
	req := launch.HandoffRequest{To: *to, Model: *model, Name: *name, Close: *closeSrc, Cols: cols, Rows: rows}
	if err := c.do("POST", "/v1/terms/"+url.PathEscape(pos[0])+"/handoff", req, &res); err != nil {
		return err
	}
	fmt.Printf("handoff   %s\nterminal  %s  (%s)\n", res.File, res.Term.Name, res.Term.Command)
	if res.Closed {
		fmt.Printf("closed    %s\n", pos[0])
	}
	if *doAttach {
		return attach(c, res.Term.Name)
	}
	fmt.Printf("attach with: fleet attach %s\n", res.Term.Name)
	return nil
}
