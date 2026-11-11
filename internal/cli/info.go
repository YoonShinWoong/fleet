package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/YoonShinWoong/keepty/internal/config"
)

// runInfo prints what another device needs to reach this daemon.
func runInfo(_ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Printf("config    %s\naddr      %s\ntoken     %s\n", config.Path(), cfg.Addr, cfg.Token)
	_, port, _ := strings.Cut(cfg.Addr, ":")
	if port == "" {
		port = "7677"
	}
	fmt.Println("\nreachable at")
	fmt.Printf("  this machine   http://127.0.0.1:%s\n", port)
	if host := firstOutput("scutil", "--get", "LocalHostName"); host != "" {
		fmt.Printf("  same wifi      http://%s.local:%s\n", host, port)
	}
	for _, ifc := range []string{"en0", "en1"} {
		if ip := firstOutput("ipconfig", "getifaddr", ifc); ip != "" {
			fmt.Printf("  lan (%s)      http://%s:%s\n", ifc, ip, port)
		}
	}
	ts := "tailscale"
	if _, err := exec.LookPath(ts); err != nil {
		ts = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"
	}
	if ip := firstOutput(ts, "ip", "-4"); ip != "" {
		fmt.Printf("  tailscale      http://%s:%s\n", ip, port)
		if dns := firstOutput("sh", "-c", ts+" status --json | sed -n 's/.*\"DNSName\": *\"\\([^\"]*\\)\\.\".*/\\1/p' | head -1"); dns != "" {
			fmt.Printf("  tailscale dns  http://%s:%s\n", dns, port)
			fmt.Printf("\nHTTPS inside your tailnet (recommended for phones): run once on this machine\n  %s serve --bg %s\nthen use https://%s/ everywhere; keepty can stay bound to loopback.\n", filepathBase(ts), port, dns)
		}
	} else {
		fmt.Println("\nNo tailscale found. For other networks install it on every device: https://tailscale.com/download")
	}
	if strings.HasPrefix(cfg.Addr, "127.") || strings.HasPrefix(cfg.Addr, "localhost") {
		fmt.Println("\nOnly loopback is bound. Without tailscale serve, set \"addr\" to \"0.0.0.0:" + port + "\" in config and restart to accept other devices.")
	}
	fmt.Printf("\non another mac\n  export KEEPTY_ADDR=<host>:%s KEEPTY_TOKEN=%s\n", port, cfg.Token)
	return nil
}

func firstOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line)
}

func filepathBase(p string) string { return filepath.Base(p) }
