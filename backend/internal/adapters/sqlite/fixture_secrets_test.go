package sqlite_test

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// repoRoot is the repository root (four levels above this package).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	require.NoError(t, err)
	return root
}

// secretPatterns loads scripts/secret-patterns.txt, the one list shared with
// scripts/check-db-secrets.py: "raw" patterns search file bytes, "value"
// patterns search app_settings values.
func secretPatterns(t *testing.T) (raw []*regexp.Regexp, value []*regexp.Regexp) {
	t.Helper()
	f, err := os.Open(filepath.Join(repoRoot(t), "scripts", "secret-patterns.txt"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		scope, pat, ok := strings.Cut(line, "\t")
		require.True(t, ok, "bad pattern line %q", line)
		re := regexp.MustCompile(pat)
		if scope == "raw" {
			raw = append(raw, re)
		} else {
			value = append(value, re)
		}
	}
	require.NoError(t, sc.Err())
	require.NotEmpty(t, raw)
	require.NotEmpty(t, value)
	return raw, value
}

// credentialProblems returns what is wrong with one fixture DB: an encrypted
// or credential-like app_settings row, a credential pattern anywhere in the
// raw bytes (a deleted row that was never VACUUMed), or a nonempty freelist.
// It never includes a secret value in a message.
func credentialProblems(t *testing.T, path string) []string {
	t.Helper()
	rawPats, valuePats := secretPatterns(t)
	var problems []string

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("SQLite format 3\x00")), "%s is not a SQLite database", path)
	for _, re := range rawPats {
		if re.Match(data) {
			problems = append(problems, fmt.Sprintf("raw bytes match credential pattern %q (deleted row not VACUUMed?)", re))
		}
	}
	if binary.BigEndian.Uint32(data[36:40]) > 0 {
		problems = append(problems, "freelist is not empty; VACUUM the fixture so deleted rows leave no residue")
	}

	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	rows, err := conn.Query(`SELECT key, value, encrypted FROM app_settings`)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return problems
	}
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, value string
		var encrypted int
		require.NoError(t, rows.Scan(&key, &value, &encrypted))
		if encrypted != 0 {
			problems = append(problems, fmt.Sprintf("app_settings %q is encrypted; remove it from the fixture", key))
		}
		for _, re := range valuePats {
			if re.MatchString(value) {
				problems = append(problems, fmt.Sprintf("app_settings %q looks like a credential (%q)", key, re))
			}
		}
	}
	require.NoError(t, rows.Err())
	return problems
}

// TestFixtureDBsCarryNoCredentials fails when any *.db in the repo (the
// tracked test fixtures) holds an encrypted app_settings row, a value that
// looks like an API key or token, or credential bytes left in the file by a
// deleted row. Fixtures are public; see .gitignore.
func TestFixtureDBsCarryNoCredentials(t *testing.T) {
	root := repoRoot(t)
	var dbs []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".worktrees":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".db") {
			dbs = append(dbs, p)
		}
		return nil
	}))
	require.NotEmpty(t, dbs)
	for _, p := range dbs {
		t.Run(filepath.Base(p), func(t *testing.T) {
			require.Empty(t, credentialProblems(t, p), p)
		})
	}
}

// TestCredentialProblemsCatchesDeletedRow proves the raw-bytes scan: a key
// that exists only in a deleted, un-VACUUMed row must still fail the check.
func TestCredentialProblemsCatchesDeletedRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.db")
	conn, err := sql.Open("sqlite", testhelper.DSNWith(path, "_pragma=journal_mode(DELETE)"))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	conn.SetMaxOpenConns(1)
	for _, q := range []string{
		`PRAGMA secure_delete=OFF`,
		`CREATE TABLE app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, encrypted INTEGER NOT NULL DEFAULT 0)`,
		// built at run time so this file itself holds no key-shaped literal
		`INSERT INTO app_settings VALUES ('ai.api_key', '` + "sk-ant-" + "api03-" + strings.Repeat("FAKE", 8) + `', 0)`,
		`DELETE FROM app_settings`,
	} {
		_, err := conn.Exec(q)
		require.NoError(t, err, q)
	}
	require.NoError(t, conn.Close())

	problems := credentialProblems(t, path)
	require.NotEmpty(t, problems, "a key in a deleted row must fail")
	require.Contains(t, strings.Join(problems, "\n"), "raw bytes")
	require.NotContains(t, strings.Join(problems, "\n"), "FAKEFAKE", "messages must not echo the value")

	conn2, err := sql.Open("sqlite", testhelper.DSNWith(path, "_pragma=journal_mode(DELETE)"))
	require.NoError(t, err)
	_, err = conn2.Exec(`VACUUM`)
	require.NoError(t, err)
	require.NoError(t, conn2.Close())
	require.Empty(t, credentialProblems(t, path), "after VACUUM the file is clean")
}
