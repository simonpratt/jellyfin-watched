// Command jellyfin-watched serves Spool, a read-only dashboard showing which
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

	handler, err := web.NewHandler(cache, artwork.New(client), logger)
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
	logger.Info("listening", "addr", ln.Addr().String(), "jellyfin_url", cfg.JellyfinURL)
	if onListen != nil {
		onListen(ln.Addr())
	}

	// Initial load in the background so the page is served straight away.
	// Scheduled and on-demand refreshes arrive in ticket 07.
	go func() {
		start := time.Now()
		if err := cache.Refresh(ctx); err != nil {
			logger.Error("library refresh failed", "err", err)
			return
		}
		snap, _ := cache.Snapshot()
		logger.Info("library loaded", "movies", len(snap.Movies), "series", len(snap.Series), "took", time.Since(start).Round(time.Millisecond))
	}()

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
