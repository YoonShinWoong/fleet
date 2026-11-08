package cli

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/YoonShinWoong/fleet/internal/config"
	"github.com/YoonShinWoong/fleet/internal/daemon"
	"github.com/YoonShinWoong/fleet/internal/state"
)

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address (overrides config)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		cfg.Token = hex.EncodeToString(b)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("wrote %s with a fresh token. Remote clients: export FLEET_TOKEN=%s\n", config.Path(), cfg.Token)
	} else if !config.Exists() {
		_ = cfg.Save()
	}
	if *addr != "" {
		cfg.Addr = *addr
	}
	store, err := state.Open(config.StatePath())
	if err != nil {
		return err
	}
	srv, err := daemon.New(cfg, store)
	if err != nil {
		return err
	}
	return srv.ListenAndServe()
}
