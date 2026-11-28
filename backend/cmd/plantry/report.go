package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/config"
	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/report"
)

// reportHelp is the report command's help text.
const reportHelp = `Usage: plantry report -out <file> [-days N]

Writes a Markdown report of how the app ran over the last N days (default 30)
to <file>, which must not exist: AI runs by kind (errors, durations, cost),
Chef spend against the monthly cap, how Chef help, finds, tips and import
checks ended, planning violations, ratings and a short list of problems.
It opens the database read-only, so it is safe while the server runs. It holds
counts, rates, durations and dish names only: no keys, prompts or text you
typed. Hand the file over; "plantry backup" is the way to hand over the data.
`

// runReport implements `plantry report -out <file> [-days N]` (see reportHelp).
func runReport(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = io.WriteString(stderr, reportHelp)
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "path of the report file (must not exist)")
	days := fs.Int("days", 30, "length of the window in days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("report: -out <file> is required")
	}
	if *days < 1 {
		return errors.New("report: -days must be at least 1")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	md, err := buildReport(context.Background(), cfg.DBPath, *days, time.Now(), settingsEnv(cfg))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("report: %w", err)
	}
	if _, err := io.WriteString(f, md); err != nil {
		_ = f.Close()
		_ = os.Remove(*out)
		return fmt.Errorf("report: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(*out)
		return fmt.Errorf("report: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "report written to %s\n", *out)
	return nil
}

// buildReport renders the report of the database at path.
func buildReport(ctx context.Context, path string, days int, now time.Time, env settingsdom.EnvSnapshot) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("report: database: %w", err)
	}
	conn, err := openReadOnlyDB(path)
	if err != nil {
		return "", fmt.Errorf("report: open %s: %w", path, err)
	}
	defer func() { _ = conn.Close() }()
	data, err := sqlite.NewReportRepo(conn).Collect(ctx, sqlite.ReportOpts{
		Now: now, Days: days, Version: version, DBBytes: info.Size(), Env: env,
	})
	if err != nil {
		return "", fmt.Errorf("report: %w", err)
	}
	return report.Render(data), nil
}

// openReadOnlyDB opens path without the ability to write (a WAL database
// can still be read while the server writes it).
func openReadOnlyDB(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
