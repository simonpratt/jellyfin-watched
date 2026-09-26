// Command jellyfin-watched serves a read-only dashboard showing which
// Jellyfin users have completed each movie and TV season.
//
// All configuration comes from environment variables (REQUIREMENTS.md §8).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	// The stale-data notice gives a clock time ("Last updated today at
	// 11:42") in the TZ environment variable's zone; the distroless image
	// has no zoneinfo of its own.
	_ "time/tzdata"

	"jellyfin-watched/internal/artwork"
	"jellyfin-watched/internal/config"
	"jellyfin-watched/internal/jellyfin"
	"jellyfin-watched/internal/library"
	"jellyfin-watched/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, os.Stderr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "jellyfin-watched: %v\n", err)
		os.Exit(1)
	}
}

// run starts the dashboard and blocks until ctx is cancelled. onListen, if
// set, is called with the bound address once the server is listening.
func run(ctx context.Context, getenv func(string) string, logOut io.Writer, onListen func(net.Addr)) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(logOut, nil))

	client, err := jellyfin.New(cfg.JellyfinURL, cfg.APIKey.Reveal(), cfg.JellyfinTimeout)
	if err != nil {
		return err
	}
	cache := library.NewCache(client, library.Options{IncludeSpecials: cfg.IncludeSpecials})

	handler, err := web.NewHandler(cache, artwork.New(client), logger, cfg.DashboardPassword.Reveal())
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", cfg.ListenAddr, err)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	logger.Info("listening", "addr", ln.Addr().String(), "jellyfin_url", cfg.JellyfinURL, "password_required", cfg.DashboardPassword.IsSet())
	if onListen != nil {
		onListen(ln.Addr())
	}

	// Refreshes run in the background, the first straight away, so pages
	// are served (from the cache) from the start. Run returns on shutdown.
	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		cache.Run(ctx, cfg.RefreshInterval, logger)
	}()
	defer func() { <-refreshDone }()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("stopped")
	return nil
}
