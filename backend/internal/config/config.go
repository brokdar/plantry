package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port              int
	DBPath            string
	LogLevel          slog.Level
	ImagePath         string
	FDCAPIKey         string
	AIProvider        string // "openai" | "anthropic" | "fake" | ""
	AIModel           string
	AIAPIKey          string
	AIRateLimitPerMin int
	AIFakeScript      string
	DevMode           bool // exposes dev-only debug endpoints; PLANTRY_DEV_MODE
}

func Load() (Config, error) {
	cfg := Config{
		Port:              8080,
		DBPath:            "/data/plantry.db",
		LogLevel:          slog.LevelInfo,
		ImagePath:         "/data/images",
		AIRateLimitPerMin: 10,
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

	if v := os.Getenv("PLANTRY_AI_PROVIDER"); v != "" {
		switch strings.ToLower(v) {
		case "openai", "anthropic", "fake":
			cfg.AIProvider = strings.ToLower(v)
		default:
			return Config{}, fmt.Errorf("PLANTRY_AI_PROVIDER invalid: %q (want openai|anthropic|fake)", v)
		}
	}
	if v := os.Getenv("PLANTRY_AI_MODEL"); v != "" {
		cfg.AIModel = v
	}
	if v := os.Getenv("PLANTRY_AI_API_KEY"); v != "" {
		cfg.AIAPIKey = v
	}
	if v := os.Getenv("PLANTRY_AI_RATE_LIMIT_PER_MIN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("PLANTRY_AI_RATE_LIMIT_PER_MIN invalid: %q", v)
		}
		cfg.AIRateLimitPerMin = n
	}
	if v := os.Getenv("PLANTRY_AI_FAKE_SCRIPT"); v != "" {
		cfg.AIFakeScript = v
	}

	if v := os.Getenv("PLANTRY_DEV_MODE"); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			cfg.DevMode = true
		case "0", "false", "no", "off":
			cfg.DevMode = false
		default:
			return Config{}, fmt.Errorf("PLANTRY_DEV_MODE invalid: %q", v)
		}
	}

	// Validate AI config: if a provider is set, model must also be set. API
	// key is required for openai/anthropic; fake reads its script instead.
	if cfg.AIProvider != "" {
		if cfg.AIModel == "" {
			return Config{}, fmt.Errorf("PLANTRY_AI_MODEL required when PLANTRY_AI_PROVIDER is set")
		}
		if cfg.AIProvider == "openai" || cfg.AIProvider == "anthropic" {
			if cfg.AIAPIKey == "" {
				return Config{}, fmt.Errorf("PLANTRY_AI_API_KEY required for %q provider", cfg.AIProvider)
			}
		}
		if cfg.AIProvider == "fake" && cfg.AIFakeScript == "" {
			return Config{}, fmt.Errorf("PLANTRY_AI_FAKE_SCRIPT required for fake provider")
		}
	}

	return cfg, nil
}
