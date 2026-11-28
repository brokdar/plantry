package main

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/pressly/goose/v3"

	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// logStartup writes the one line that says what is running: build, schema,
// Chef switch, provider and model (never a key), and the clock the app sees.
// It is the first thing to read in a log from a server nobody can open. A field
// that cannot be read is left out and named in a warning; it never stops the
// server.
func logStartup(ctx context.Context, logger *slog.Logger, conn *sql.DB, svc *settingsdom.Service, now time.Time) {
	zone, _ := now.Zone()
	attrs := []any{"version", version, "commit", commit}
	var failed []string
	warn := func(field string, err error) {
		failed = append(failed, field)
		logger.WarnContext(ctx, "server.start.incomplete", "field", field, "error", err.Error())
	}
	if schema, err := goose.GetDBVersion(conn); err != nil {
		warn("schema_version", err)
	} else {
		attrs = append(attrs, "schema_version", schema)
	}
	if chefOn, err := svc.ChefEnabled(ctx); err != nil {
		warn("chef_enabled", err)
	} else {
		attrs = append(attrs, "chef_enabled", chefOn)
	}
	if ai, err := svc.EffectiveAI(ctx); err != nil {
		warn("ai", err)
	} else {
		attrs = append(attrs, "ai_provider", ai.Provider, "ai_model", ai.Model)
	}
	attrs = append(attrs,
		"tz", zone,
		"local_time", now.Format(time.RFC3339),
		"today", now.Format(time.DateOnly),
	)
	logger.InfoContext(ctx, "server.start", attrs...)
}
