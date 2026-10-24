package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/config"
	"github.com/smford/webhook-replay-proxy/internal/server"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		configPath string
		showVer    bool
		port       int
	)

	flag.StringVar(&configPath, "config", "", "Path to YAML configuration file")
	flag.BoolVar(&showVer, "version", false, "Print version information and exit")
	flag.IntVar(&port, "port", 0, "HTTP server port (overrides config)")
	flag.Parse()

	if showVer {
		fmt.Printf("webhook-replay-proxy version %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	if port > 0 {
		cfg.Server.Port = port
	}

	srvInstance := server.NewServer(cfg, version)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srvInstance.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Starting webhook-replay-proxy %s on http://%s", version, addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Fatalf("Server failed to start: %v", err)
	case sig := <-shutdown:
		log.Printf("Shutdown signal %v received, stopping gracefully...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("Graceful shutdown failed: %v, forcing exit", err)
			_ = httpServer.Close()
		}
	}
}
