package cli

import (
	"errors"
	"flag"
	"os"

	"github.com/YoonShinWoong/fleet/internal/terminal"
)

func runSh(args []string) error {
	fs := flag.NewFlagSet("sh", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "working directory (default: current)")
	cmd := fs.String("c", "", "command to run instead of a shell")
	noAttach := fs.Bool("d", false, "create detached, do not attach")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: fleet sh <name> [-cwd dir] [-c command] [-d]")
	}
	name := pos[0]
	if *cwd == "" {
		*cwd, _ = os.Getwd()
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	cols, rows := termSize()
	var info terminal.Info
	body := map[string]any{"name": name, "cwd": *cwd, "command": *cmd, "cols": cols, "rows": rows}
	if err := c.do("POST", "/v1/terms", body, &info); err != nil {
		return err
	}
	if *noAttach {
		return nil
	}
	return attach(c, name)
}
