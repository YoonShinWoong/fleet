package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Printf("shutting down: saving terminals")
		srv.Shutdown()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
