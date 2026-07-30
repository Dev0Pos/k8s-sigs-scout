// Command k8s-scout runs the kubernetes-sigs good-first-issue dashboard.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/github"
	"k8s-scout/internal/logging"
	"k8s-scout/internal/server"
)

func main() {
	slog.SetDefault(logging.NewFromEnv())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	auth := github.ConfigureDefaultFromEnv()
	// Never log the token value — only whether auth is enabled.
	slog.Info("github api auth", "enabled", auth)

	c := &cache.Cache{}
	cache.StartRefresher(c, cache.DefaultInterval)

	srv, err := server.New(c)
	if err != nil {
		slog.Error("server init failed", "err", err)
		os.Exit(1)
	}

	addr := ":" + port
	slog.Info("listening", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
