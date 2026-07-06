package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"im-go/internal/integrationhub"
)

type serverConfig struct {
	Addr string
}

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           buildHandler(time.Now().UTC()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("starting integration API", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}

func buildHandler(now time.Time) http.Handler {
	store := integrationhub.NewStore(integrationhub.SeedSnapshot(now))
	store.SetClock(func() time.Time { return time.Now().UTC() })
	return integrationhub.NewHandler(store)
}

func loadConfig() serverConfig {
	addr := strings.TrimSpace(os.Getenv("IM_API_ADDR"))
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv("ADDR"))
	}
	if addr == "" {
		addr = ":9000"
	}
	return serverConfig{Addr: addr}
}
