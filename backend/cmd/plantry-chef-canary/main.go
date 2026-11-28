// Command plantry-chef-canary is the paid canary of the Chef's attribute
// labelling (A3): it labels the unlabelled recipes of a database with the
// configured provider and compares them with the DATA-1 labels.
//
//	# scripted, no network:
//	go run ./cmd/plantry-chef-canary -db /tmp/canary.db -fake-script labels.json
//	# live, costs money:
//	PLANTRY_CHEF_CANARY_LIVE=1 PLANTRY_AI_PROVIDER=anthropic PLANTRY_AI_API_KEY=... \
//	  go run ./cmd/plantry-chef-canary -db /tmp/canary.db -model <model>
//	# live, provider/model/key from a (copy of a) server database:
//	PLANTRY_CHEF_CANARY_LIVE=1 go run ./cmd/plantry-chef-canary -db /tmp/canary.db \
//	  -settings-db /path/plantry.db [-secret-key-file /path/secret.key]
//
// -mode picks what runs (labels is the default, the A3 labelling above):
//
//	finds        the real find pipeline over 40 requests (themes x constraints)
//	importcheck  planted-error and clean recipes through the import check
//	subs         substitution cases (rule-based, no provider needed)
//	help         explain-step and went-wrong questions through chefhelp
//	month        a normal month's cost computed from the run log
//
// Live runs of finds, importcheck and help need -spend-cap (USD): the run stops
// before the next case once the run log's cost of the run reaches it. Each mode
// writes -out as JSON and prints one line per metric. Use a copy of a real
// database: finds, runs and the labels are written into it.
//
// A missing -db file is created, migrated and seeded with the set; an
// existing one is labelled as it is (use a copy: the labels and the run log
// are written into it). Every call is in that database's run log.
package main

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/openai"
	"github.com/jaltszeimer/plantry/backend/internal/chefcanary"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
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
	settingsDB := fs.String("settings-db", "", "read the AI provider, model and key from this server database (opened read-only); env vars win")
	secretKeyFile := fs.String("secret-key-file", "", "secret key file that decrypts -settings-db (default PLANTRY_SECRET_KEY, else secret.key next to it)")
	out := fs.String("out", "", "write the JSON report here")
	mode := fs.String("mode", "labels", "labels, finds, importcheck, subs, help or month")
	spendCap := fs.Float64("spend-cap", 0, "USD: stop before the next case once the run log's cost of this run reaches it (required for a live finds, importcheck or help run)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return errors.New("-db is required")
	}
	if !slices.Contains(modes, *mode) {
		return fmt.Errorf("-mode must be one of %s", strings.Join(modes, ", "))
	}
	if *spendCap < 0 {
		return errors.New("-spend-cap must not be negative")
	}
	if *mode != "labels" && *mode != "subs" && *mode != "month" && *fakeScript == "" && *spendCap == 0 {
		return fmt.Errorf("a live %s run needs -spend-cap", *mode)
	}
	set, err := loadSet(*sealed)
	if err != nil {
		return err
	}
	var (
		client       llm.Client
		providerName string
		mdl          string
	)
	if *mode != "subs" && *mode != "month" {
		if client, providerName, mdl, err = newClient(getenv, *model, *fakeScript, *settingsDB, *secretKeyFile); err != nil {
			return err
		}
	}
	conn, err := openDB(*dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if *mode != "month" {
		if err := chefcanary.Seed(ctx, conn, set); err != nil {
			return err
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	var rep report
	if *mode == "labels" {
		rep, err = chefcanary.Run(ctx, chefcanary.Config{DB: conn, Set: set, Client: client, Provider: providerName, Model: mdl, Logger: logger})
	} else {
		rep, err = runMode(ctx, *mode, chefcanary.Env{DB: conn, Client: client, Provider: providerName, Model: mdl, Logger: logger, SpendCap: *spendCap}, *fakeScript != "")
	}
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
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		return err
	}
	if a, ok := rep.(interface{ ArtifactFiles() map[string]string }); ok {
		for name, content := range a.ArtifactFiles() {
			if err := os.WriteFile(filepath.Join(filepath.Dir(*out), name), []byte(content), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// modes are the values of -mode.
var modes = []string{"labels", "finds", "importcheck", "subs", "help", "month"}

// report is what a mode returns: a JSON document and a text summary.
type report interface{ Summary() string }

// runMode runs one of the measurement modes. A scripted run reads no pages
// (the finds fetcher refuses), so it never touches the network.
func runMode(ctx context.Context, mode string, env chefcanary.Env, scripted bool) (report, error) {
	switch mode {
	case "finds":
		reqs, err := chefsets.LoadFinds()
		if err != nil {
			return nil, err
		}
		var fetcher finds.Fetcher = scriptedFetcher{}
		if !scripted {
			fetcher = httpfetch.NewPolite(httpfetch.New(httpfetch.WithDialGuard(nil, false)))
		}
		return chefcanary.RunFinds(ctx, chefcanary.FindsConfig{Env: env, Requests: reqs, Fetcher: fetcher})
	case "importcheck":
		set, err := chefsets.LoadImportCheck()
		if err != nil {
			return nil, err
		}
		return chefcanary.RunImportCheck(ctx, chefcanary.ImportCheckConfig{Env: env, Set: set})
	case "subs":
		cases, err := chefsets.LoadSubs()
		if err != nil {
			return nil, err
		}
		return chefcanary.RunSubs(ctx, chefcanary.SubsConfig{DB: env.DB, Cases: cases})
	case "help":
		set, err := chefsets.LoadHelp()
		if err != nil {
			return nil, err
		}
		return chefcanary.RunHelp(ctx, chefcanary.HelpConfig{Env: env, Set: set})
	}
	return chefcanary.RunMonth(ctx, chefcanary.MonthConfig{DB: env.DB})
}

// scriptedFetcher is the finds fetcher of a scripted run: no pages.
type scriptedFetcher struct{}

func (scriptedFetcher) Fetch(context.Context, string) (string, string, error) {
	return "", "", errors.New("a scripted run reads no pages")
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
// provider from the environment (or -settings-db), which needs
// PLANTRY_CHEF_CANARY_LIVE=1.
func newClient(getenv func(string) string, model, fakeScript, settingsDB, secretKeyFile string) (llm.Client, string, string, error) {
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
	if settingsDB != "" && (p == "" || key == "" || model == "") {
		stored, err := readStoredAI(settingsDB, secretKeyFile, getenv("PLANTRY_SECRET_KEY"))
		if err != nil {
			return nil, "", "", err
		}
		p, key, model = cmp.Or(p, stored[settings.KeyAIProvider]), cmp.Or(key, stored[settings.KeyAIAPIKey]), cmp.Or(model, stored[settings.KeyAIModel])
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

// readStoredAI returns the ai.provider, ai.model and ai.api_key rows of a
// server database, decrypted with the secret key the server uses: the env
// value, else the key file (default: secret.key next to the database). The
// database is opened read-only and the key never printed.
func readStoredAI(dbPath, keyFile, envSecret string) (map[string]string, error) {
	secret := envSecret
	if secret == "" {
		if keyFile == "" {
			keyFile = filepath.Join(filepath.Dir(dbPath), "secret.key")
		}
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("secret key: set PLANTRY_SECRET_KEY or -secret-key-file: %w", err)
		}
		secret = strings.TrimSpace(string(b))
	}
	cipher, err := crypto.New(secret)
	if err != nil {
		return nil, fmt.Errorf("secret key: %w", err)
	}
	conn, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	rows, err := conn.Query(`SELECT key, value, encrypted FROM app_settings WHERE key IN (?, ?, ?)`,
		settings.KeyAIProvider, settings.KeyAIModel, settings.KeyAIAPIKey)
	if err != nil {
		return nil, fmt.Errorf("read settings from %s: %w", dbPath, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		var enc bool
		if err := rows.Scan(&k, &v, &enc); err != nil {
			return nil, err
		}
		if enc {
			plain, err := cipher.Decrypt(v)
			if err != nil {
				return nil, fmt.Errorf("decrypt %s (wrong secret key?): %w", k, err)
			}
			v = string(plain)
		}
		out[k] = v
	}
	return out, rows.Err()
}
