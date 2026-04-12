package config_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PLANTRY_PORT", "")
	t.Setenv("PLANTRY_DB_PATH", "")
	t.Setenv("PLANTRY_LOG_LEVEL", "")
	t.Setenv("PLANTRY_IMAGE_PATH", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "/data/plantry.db", cfg.DBPath)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.Equal(t, "/data/images", cfg.ImagePath)
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("PLANTRY_PORT", "9090")
	t.Setenv("PLANTRY_DB_PATH", "/tmp/t.db")
	t.Setenv("PLANTRY_LOG_LEVEL", "debug")
	t.Setenv("PLANTRY_IMAGE_PATH", "/tmp/images")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "/tmp/t.db", cfg.DBPath)
	assert.Equal(t, slog.LevelDebug, cfg.LogLevel)
	assert.Equal(t, "/tmp/images", cfg.ImagePath)
}

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("PLANTRY_PORT", "abc")
	_, err := config.Load()
	assert.Error(t, err)
}
