package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/YoonShinWoong/fleet/internal/config"
)

const serviceLabel = "dev.fleet.daemon"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
}

// servicePlist renders a launchd agent that keeps `fleet daemon` running for
// the logged-in user: starts at login, restarts if it dies, logs to ~/.fleet.
func servicePlist(bin, home string) string {
	logPath := filepath.Join(config.Dir(), "daemon.log")
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + serviceLabel + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + bin + `</string>
    <string>daemon</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ProcessType</key><string>Interactive</string>
  <key>WorkingDirectory</key><string>` + home + `</string>
  <key>StandardOutPath</key><string>` + logPath + `</string>
  <key>StandardErrorPath</key><string>` + logPath + `</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>HOME</key><string>` + home + `</string>
    <key>FLEET_HOME</key><string>` + config.Dir() + `</string>
  </dict>
</dict>
</plist>
`
}

func runService(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fleet service install | uninstall | status | restart | log | plist")
	}
	home, _ := os.UserHomeDir()
	uid := fmt.Sprint(os.Getuid())
	target := "gui/" + uid + "/" + serviceLabel
	switch args[0] {
	case "plist":
		bin, _ := os.Executable()
		bin, _ = filepath.EvalSymlinks(bin)
		fmt.Print(servicePlist(bin, home))
		return nil
	case "install":
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		bin, _ = filepath.EvalSymlinks(bin)
		if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(plistPath(), []byte(servicePlist(bin, home)), 0o644); err != nil {
			return err
		}
		_ = exec.Command("launchctl", "bootout", target).Run()
		if out, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath()).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
		}
		_ = exec.Command("launchctl", "kickstart", "-k", target).Run()
		fmt.Printf("installed %s\nbinary    %s\nlog       %s\n\nIt starts at login and restarts if it dies. `fleet service status` to check, `fleet info` for the address and token.\n", plistPath(), bin, filepath.Join(config.Dir(), "daemon.log"))
		return nil
	case "uninstall":
		_ = exec.Command("launchctl", "bootout", target).Run()
		if err := os.Remove(plistPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Println("removed", plistPath())
		return nil
	case "restart":
		if out, err := exec.Command("launchctl", "kickstart", "-k", target).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl kickstart: %v: %s (is the service installed?)", err, strings.TrimSpace(string(out)))
		}
		fmt.Println("restarted")
		return nil
	case "status":
		out, err := exec.Command("launchctl", "print", target).CombinedOutput()
		if err != nil {
			fmt.Println("not installed (fleet service install)")
			return nil
		}
		for _, line := range strings.Split(string(out), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "state = ") || strings.HasPrefix(l, "pid = ") || strings.HasPrefix(l, "last exit code = ") || strings.HasPrefix(l, "runs = ") {
				fmt.Println(l)
			}
		}
		return nil
	case "log":
		cmd := exec.Command("tail", "-n", "50", filepath.Join(config.Dir(), "daemon.log"))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("unknown service command %q", args[0])
}
