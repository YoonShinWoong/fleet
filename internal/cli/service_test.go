package cli

import (
	"strings"
	"testing"
)

func TestServicePlist(t *testing.T) {
	p := servicePlist("/usr/local/bin/fleet", "/Users/me")
	for _, want := range []string{"<string>/usr/local/bin/fleet</string>", "<string>daemon</string>", "<key>KeepAlive</key><true/>", "<key>RunAtLoad</key><true/>", "<key>HOME</key><string>/Users/me</string>"} {
		if !strings.Contains(p, want) {
			t.Fatalf("plist lacks %s", want)
		}
	}
}
