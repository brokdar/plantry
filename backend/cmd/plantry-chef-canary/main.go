// Command plantry-chef-canary is the paid canary of the Chef's attribute
// labelling (A3): it labels the unlabelled recipes of a database with the
// configured provider and compares them with the DATA-1 labels.
//
//	# scripted, no network:
//	go run ./cmd/plantry-chef-canary -db /tmp/canary.db -fake-script labels.json
//	# live, costs money:
//	PLANTRY_CHEF_CANARY_LIVE=1 PLANTRY_AI_PROVIDER=anthropic PLANTRY_AI_API_KEY=... \
//	  go run ./cmd/plantry-chef-canary -db /tmp/canary.db -model <model>
//
// A missing -db file is created, migrated and seeded with the set; an
// existing one is labelled as it is (use a copy: the labels and the run log
// are written into it). Every call is in that database's run log.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/chefcanary"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "plantry-chef-canary:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	fs := flag.NewFlagSet("plantry-chef-canary", flag.ContinueOnError)
	dbPath := fs.String("db", "", "database to label (created and seeded with the set when missing)")
	sealed := fs.Bool("sealed", false, "use the sealed set too (PLANTRY_CHEF_SETS_SEALED)")
	model := fs.String("model", "", "model (default PLANTRY_AI_MODEL)")
	fakeScript := fs.String("fake-script", "", "scripted run with the fake provider, no network")
	out := fs.String("out", "", "write the JSON report here")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return errors.New("-db is required")
	}
	set, err := loadSet(*sealed)
	if err != nil {
		return err
	}
	client, providerName, mdl, err := newClient(getenv, *model, *fakeScript)
	if err != nil {
		return err
	}
	conn, err := openDB(*dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := chefcanary.Seed(ctx, conn, set); err != nil {
		return err
	}
	rep, err := chefcanary.Run(ctx, chefcanary.Config{
		DB: conn, Set: set, Client: client, Provider: providerName, Model: mdl,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(stdout, rep.Summary()); err != nil {
		return err
	}
	if *out == "" {
		return nil
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, b, 0o600)
}

func loadSet(sealed bool) (chefsets.Set, error) {
	if !sealed {
		return chefsets.LoadDev()
	}
	set, ok, err := chefsets.LoadSealed()
	if err == nil && !ok {
		err = errors.New("-sealed needs PLANTRY_CHEF_SETS_SEALED")
	}
	return set, err
}

// newClient builds the client: the fake with -fake-script, else the real
// provider from the environment, which needs PLANTRY_CHEF_CANARY_LIVE=1.
func newClient(getenv func(string) string, model, fakeScript string) (llm.Client, string, string, error) {
	if fakeScript != "" {
		c, err := fake.New(fakeScript)
		return c, "fake", "fake-canary", err
	}
	if getenv("PLANTRY_CHEF_CANARY_LIVE") != "1" {
		return nil, "", "", errors.New("a live run costs money: set PLANTRY_CHEF_CANARY_LIVE=1, or pass -fake-script")
	}
	p, key := getenv("PLANTRY_AI_PROVIDER"), getenv("PLANTRY_AI_API_KEY")
	if model == "" {
		model = getenv("PLANTRY_AI_MODEL")
	}
	if key == "" || model == "" {
		return nil, "", "", errors.New("live mode needs PLANTRY_AI_API_KEY and -model or PLANTRY_AI_MODEL")
	}
	switch p {
	case "anthropic":
		return anthropic.New(key), p, model, nil
	case "openai":
		return openai.New(key), p, model, nil
	}
	return nil, "", "", fmt.Errorf("live mode needs PLANTRY_AI_PROVIDER anthropic or openai, not %q", p)
}

// openDB opens and migrates the database like the server does.
func openDB(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	if err = goose.SetDialect("sqlite3"); err == nil {
		err = goose.Up(conn, "migrations")
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return conn, nil
}
