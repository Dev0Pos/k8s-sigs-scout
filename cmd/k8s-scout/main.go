// Command k8s-scout runs the kubernetes-sigs good-first-issue dashboard.
package main

import (
	"log"
	"net/http"
	"os"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/github"
	"k8s-scout/internal/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if github.ConfigureDefaultFromEnv() {
		log.Printf("GitHub API auth: enabled (GITHUB_TOKEN set)")
	} else {
		log.Printf("GitHub API auth: disabled (unauthenticated rate limits)")
	}

	c := &cache.Cache{}
	cache.StartRefresher(c, cache.DefaultInterval)

	srv, err := server.New(c)
	if err != nil {
		log.Fatalf("server init: %v", err)
	}

	addr := ":" + port
	log.Printf("k8s-sigs-scout listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
