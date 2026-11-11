package cli

import "fmt"

func runMCP(_ []string) error {
	fmt.Println("keepty mcp: planned for v0.2. Source of truth: registry/mcp.yaml in this repo.")
	fmt.Println("  render  write Claude user-scope servers, Codex config.toml [mcp_servers], and repo .mcp.json")
	fmt.Println("  doctor  probe each server from this host and disable intranet-tagged ones that do not resolve")
	return nil
}

func runMemory(_ []string) error {
	fmt.Println("keepty memory: planned for v0.2. One OKF bundle shared by Claude (autoMemoryDirectory) and Codex (AGENTS.md).")
	fmt.Println("  check   validate frontmatter and rebuild MEMORY.md index")
	fmt.Println("  merge   fold the per-project bundles under ~/.claude/projects/*/memory into the shared one")
	return nil
}
