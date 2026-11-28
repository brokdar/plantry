package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/pressly/goose/v3"

	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// logStartup writes the one line that says what is running: build, schema,
// Chef switch, provider and model (never a key), and the clock the app sees.
// It is the first thing to read in a log from a server nobody can open.
func logStartup(ctx context.Context, logger *slog.Logger, conn *sql.DB, svc *settingsdom.Service, now time.Time) error {
	schema, err := goose.GetDBVersion(conn)
	if err != nil {
		return fmt.Errorf("schema version: %w", err)
	}
	chefOn, err := svc.ChefEnabled(ctx)
	if err != nil {
		return fmt.Errorf("chef enabled: %w", err)
	}
	ai, err := svc.EffectiveAI(ctx)
	if err != nil {
		return fmt.Errorf("effective ai: %w", err)
	}
	zone, _ := now.Zone()
	logger.InfoContext(ctx, "server.start",
		"version", version,
		"commit", commit,
		"schema_version", schema,
		"chef_enabled", chefOn,
		"ai_provider", ai.Provider,
		"ai_model", ai.Model,
		"tz", zone,
		"local_time", now.Format(time.RFC3339),
		"today", now.Format(time.DateOnly),
	)
	return nil
}
