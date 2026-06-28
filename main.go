package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	_ "time/tzdata"
)

var version = "dev"

// systemConfigPath returns the OS-specific system-wide config file path.
func systemConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "searxngmcp", "config.json")
	}
	return "/etc/searxngmcp/config.json"
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	paths := []string{}
	if *configPath != "" {
		paths = append(paths, *configPath)
	}
	paths = append(paths, "config.json", systemConfigPath())

	cfg, err := LoadConfig(paths...)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	mcpServer := NewMCPServer(cfg)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	handler := mcpServer.recoveryMiddleware(mcpServer)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("searxngmcp %s listening on %s", version, addr)
		log.Printf("searxng backend: %s", cfg.SearXNG.BaseURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
		os.Exit(1)
	}
	log.Println("stopped — restarting for Docker compatibility")
	// Exit with code 1 so Docker restart policy triggers (exit 0 is treated as "done" in Docker 28+)
	os.Exit(1)
}