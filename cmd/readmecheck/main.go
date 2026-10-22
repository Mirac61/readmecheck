package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"readmecheck/internal/web"
)

func main() {
	addr := "localhost:8080"
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		addr = "localhost:" + strings.TrimPrefix(port, ":")
	}

	server, err := web.New(web.Config{
		AssetsDir:     env("ASSETS_DIR", "assets"),
		DatabasePath:  env("DATABASE_PATH", "readmecheck.db"),
		GitHubToken:   strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubAPIURL:  strings.TrimSpace(os.Getenv("GITHUB_API_URL")),
		ExternalLinks: envBool("EXTERNAL_LINKS", true),
	})
	if err != nil {
		slog.Error("create server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("readmecheck listening", "url", "http://"+addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "server: %v\n", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "error", err)
	}
	if err := server.Close(); err != nil {
		slog.Error("close database", "error", err)
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("ignoring invalid boolean environment variable", "key", key, "value", value)
		return fallback
	}
	return parsed
}
