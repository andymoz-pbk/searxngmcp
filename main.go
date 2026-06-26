package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

	log.Printf("searxngmcp %s listening on %s", version, addr)
	log.Printf("searxng backend: %s", cfg.SearXNG.BaseURL)

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}