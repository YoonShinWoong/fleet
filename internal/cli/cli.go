// Package cli implements the keepty command line.
package cli

import (
	"fmt"
	"os"

	"github.com/YoonShinWoong/keepty/internal/version"
)

const usageText = `keepty, a lightweight tmux for coding agents, reachable from any device

usage: keepty <command> [args]

  daemon [-addr host:port]          run the control plane on the always-on host
  service install|uninstall|status|restart|log
                                    keep the daemon running via launchd (starts at login, restarts on crash)
  info                              address, token and URLs other devices should use
  ws ls | new <name> -repo r|-path p | rm <name> [-delete]
                                    workspaces: a git worktree per task, or any folder
  open <ws> [-agent shell|claude|codex] [-name n] [-a]
                                    start a terminal in a workspace
  new <repo> <task> [-agent a] [-a] shortcut: workspace <task> from <repo> + an agent in it
  sh <name> [-cwd dir] [-c cmd]     start a plain terminal outside any workspace and attach
  attach <name>                     attach to a terminal (detach: Ctrl-\)
  model <name> <model>              switch a running agent's model (types its /model command)
  handoff <name> -to <agent> [-model m] [-close] [-a]
                                    hand the work to another agent in the same place
  ls [-a]                           list terminals with agent status (-a includes ended sessions)
  send <name> <text...>             type a line into a terminal
  search <text>                     find text in every terminal's scrollback
  kill <name>                       end a terminal's process group and forget it
  hooks install|show claude|codex   wire "keepty hook" into the agent's hook config (keeps existing hooks)
  hook <claude|codex>               hook entrypoint: forwards stdin JSON to the daemon, prints {}
  mcp ..., memory ...               (v0.2)
  version

environment:
  KEEPTY_HOME    config and state dir (default ~/.keepty)
  KEEPTY_ADDR    daemon address for clients (default from config, 127.0.0.1:7677)
  KEEPTY_TOKEN   bearer token for non-loopback daemons (default from local config)
`

func Run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "daemon":
		return runDaemon(args[1:])
	case "service":
		return runService(args[1:])
	case "info":
		return runInfo(args[1:])
	case "ws", "workspace":
		return runWS(args[1:])
	case "open":
		return runOpen(args[1:])
	case "search":
		return runSearch(args[1:])
	case "model":
		return runModel(args[1:])
	case "handoff":
		return runHandoff(args[1:])
	case "sh":
		return runSh(args[1:])
	case "new":
		return runNew(args[1:])
	case "attach", "a":
		return runAttach(args[1:])
	case "ls":
		return runLs(args[1:])
	case "send":
		return runSend(args[1:])
	case "kill", "rm":
		return runKill(args[1:])
	case "hook":
		return runHook(args[1:])
	case "hooks":
		return runHooks(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "memory":
		return runMemory(args[1:])
	case "version", "-v", "--version":
		fmt.Println("keepty", version.Version)
		return nil
	case "help", "-h", "--help":
		return usage()
	}
	return fmt.Errorf("unknown command %q (try: keepty help)", args[0])
}

func usage() error {
	fmt.Fprint(os.Stdout, usageText)
	return nil
}
