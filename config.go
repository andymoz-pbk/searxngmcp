package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Server  ServerConfig  `json:"server"`
	SearXNG SearXNGConfig `json:"searxng"`
	Search  SearchConfig  `json:"search"`
	Fetch   FetchConfig   `json:"fetch"`
	Logging LoggingConfig `json:"logging"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SearXNGConfig struct {
	BaseURL string `json:"base_url"`
	Timeout int    `json:"timeout"`
}

type SearchConfig struct {
	DefaultMaxResults int    `json:"default_max_results"`
	MaxMaxResults     int    `json:"max_max_results"`
	DefaultCategories string `json:"default_categories"`
	DefaultLanguage   string `json:"default_language"`
	DefaultSafesearch int    `json:"default_safesearch"`
}

type FetchConfig struct {
	MaxContentLength int    `json:"max_content_length"`
	Timeout          int    `json:"timeout"`
	UserAgent        string `json:"user_agent"`
	MaxConcurrent    int    `json:"max_concurrent"`
}

type LoggingConfig struct {
	Level string `json:"level"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8000,
		},
		SearXNG: SearXNGConfig{
			BaseURL: "http://localhost:8080",
			Timeout: 30,
		},
		Search: SearchConfig{
			DefaultMaxResults: 10,
			MaxMaxResults:     50,
			DefaultCategories: "general",
			DefaultLanguage:   "",
			DefaultSafesearch: 0,
		},
		Fetch: FetchConfig{
			MaxContentLength: 1_048_576,
			Timeout:          30,
			UserAgent:        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
			MaxConcurrent:    5,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

func LoadConfig(paths ...string) (*Config, error) {
	cfg := DefaultConfig()

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading config %s: %w", path, err)
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	applyEnvOverrides(cfg)

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	cfg.Server.Host = envStr("SEARXNGMCP_SERVER_HOST", cfg.Server.Host)
	cfg.Server.Port = envInt("SEARXNGMCP_SERVER_PORT", cfg.Server.Port)
	cfg.SearXNG.BaseURL = envStr("SEARXNGMCP_SEARXNG_BASE_URL", cfg.SearXNG.BaseURL)
	cfg.SearXNG.Timeout = envInt("SEARXNGMCP_SEARXNG_TIMEOUT", cfg.SearXNG.Timeout)
	cfg.Search.DefaultMaxResults = envInt("SEARXNGMCP_SEARCH_DEFAULT_MAX_RESULTS", cfg.Search.DefaultMaxResults)
	cfg.Search.MaxMaxResults = envInt("SEARXNGMCP_SEARCH_MAX_MAX_RESULTS", cfg.Search.MaxMaxResults)
	cfg.Search.DefaultCategories = envStr("SEARXNGMCP_SEARCH_DEFAULT_CATEGORIES", cfg.Search.DefaultCategories)
	cfg.Search.DefaultLanguage = envStr("SEARXNGMCP_SEARCH_DEFAULT_LANGUAGE", cfg.Search.DefaultLanguage)
	cfg.Search.DefaultSafesearch = envInt("SEARXNGMCP_SEARCH_DEFAULT_SAFESEARCH", cfg.Search.DefaultSafesearch)
	cfg.Fetch.MaxContentLength = envInt("SEARXNGMCP_FETCH_MAX_CONTENT_LENGTH", cfg.Fetch.MaxContentLength)
	cfg.Fetch.Timeout = envInt("SEARXNGMCP_FETCH_TIMEOUT", cfg.Fetch.Timeout)
	cfg.Fetch.UserAgent = envStr("SEARXNGMCP_FETCH_USER_AGENT", cfg.Fetch.UserAgent)
	cfg.Fetch.MaxConcurrent = envInt("SEARXNGMCP_FETCH_MAX_CONCURRENT", cfg.Fetch.MaxConcurrent)
	cfg.Logging.Level = envStr("SEARXNGMCP_LOGGING_LEVEL", cfg.Logging.Level)
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
