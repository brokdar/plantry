package sqlite_test

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

var credentialLike = regexp.MustCompile(`sk-|ghp_|github_pat_|AKIA`)

// TestFixtureDBsCarryNoCredentials fails when any *.db in the repo (the
// tracked test fixtures) holds an encrypted app_settings row or a value that
// looks like an API key or token. Fixtures are public; see .gitignore.
func TestFixtureDBsCarryNoCredentials(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	require.NoError(t, err)
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
			conn, err := sql.Open("sqlite", "file:"+p+"?mode=ro&immutable=1")
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			rows, err := conn.Query(`SELECT key, value, encrypted FROM app_settings`)
			if err != nil && strings.Contains(err.Error(), "no such table") {
				return
			}
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var key, value string
				var encrypted int
				require.NoError(t, rows.Scan(&key, &value, &encrypted))
				require.Zero(t, encrypted, "%s: app_settings %q is encrypted; remove it from the fixture", p, key)
				require.False(t, credentialLike.MatchString(value), "%s: app_settings %q looks like a credential", p, key)
			}
			require.NoError(t, rows.Err())
		})
	}
}
