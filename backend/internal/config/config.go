package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port      int
	DBPath    string
	LogLevel  slog.Level
	ImagePath string
	FDCAPIKey string
}

func Load() (Config, error) {
	cfg := Config{
		Port:      8080,
		DBPath:    "/data/plantry.db",
		LogLevel:  slog.LevelInfo,
		ImagePath: "/data/images",
	}

	if v := os.Getenv("PLANTRY_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return Config{}, fmt.Errorf("PLANTRY_PORT invalid: %q", v)
		}
		cfg.Port = p
	}

	if v := os.Getenv("PLANTRY_DB_PATH"); v != "" {
		cfg.DBPath = v
	}

	if v := os.Getenv("PLANTRY_LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn", "warning":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return Config{}, fmt.Errorf("PLANTRY_LOG_LEVEL invalid: %q", v)
		}
	}

	if v := os.Getenv("PLANTRY_IMAGE_PATH"); v != "" {
		cfg.ImagePath = v
	}

	if v := os.Getenv("PLANTRY_FDC_API_KEY"); v != "" {
		cfg.FDCAPIKey = v
	}

	return cfg, nil
}
