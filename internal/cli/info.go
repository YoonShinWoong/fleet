package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/YoonShinWoong/fleet/internal/config"
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
	if ip := firstOutput("tailscale", "ip", "-4"); ip != "" {
		fmt.Printf("  tailscale      http://%s:%s\n", ip, port)
	} else if ip := firstOutput("/Applications/Tailscale.app/Contents/MacOS/Tailscale", "ip", "-4"); ip != "" {
		fmt.Printf("  tailscale      http://%s:%s\n", ip, port)
	}
	for _, ifc := range []string{"en0", "en1"} {
		if ip := firstOutput("ipconfig", "getifaddr", ifc); ip != "" {
			fmt.Printf("  lan (%s)      http://%s:%s\n", ifc, ip, port)
		}
	}
	if strings.HasPrefix(cfg.Addr, "127.") || strings.HasPrefix(cfg.Addr, "localhost") {
		fmt.Println("\nOnly loopback is bound. For other devices set \"addr\" to \"0.0.0.0:" + port + "\" (or the tailscale ip) in config and restart.")
	}
	fmt.Printf("\non another mac\n  export FLEET_ADDR=<host>:%s FLEET_TOKEN=%s\n", port, cfg.Token)
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
