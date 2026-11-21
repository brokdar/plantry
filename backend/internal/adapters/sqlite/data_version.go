package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// DataVersion is planning.Versioner over a database with one connection
// (cmd/plantry openDB and the tests set SetMaxOpenConns(1)). Its token
// changes whenever a row the connection can see may have changed:
//
//   - total_changes(): rows this connection inserted, updated or deleted,
//     inside a transaction too (a rolled-back write still moves it on);
//   - PRAGMA data_version: commits by any other connection;
//   - a random id per connection, kept in a temp table: both counters only
//     compare on the same connection, and the pool may open a new one;
//   - the unit of work: rows a transaction wrote are seen only inside it,
//     and a rollback takes them back without moving total_changes() back,
//     so a State built inside a transaction never matches outside it.
type DataVersion struct {
	conn
}

// NewDataVersion returns the data version of db.
func NewDataVersion(db *sql.DB) *DataVersion { return &DataVersion{conn: newConn(db)} }

// DataVersion implements planning.Versioner. ok is false with more than one
// connection (an uncommitted write on another one is invisible here but
// would share the counters' values) or on any error.
func (v *DataVersion) DataVersion(ctx context.Context) (string, bool) {
	if v.db.Stats().MaxOpenConnections != 1 {
		return "", false
	}
	q := v.dbtx(ctx)
	var id string
	if err := q.QueryRowContext(ctx, `SELECT id FROM temp.plantry_conn_id`).Scan(&id); err != nil {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return "", false
		}
		id = hex.EncodeToString(b)
		if _, err := q.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS plantry_conn_id (id TEXT NOT NULL)`); err != nil {
			return "", false
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM temp.plantry_conn_id`); err != nil {
			return "", false
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO temp.plantry_conn_id (id) VALUES (?)`, id); err != nil {
			return "", false
		}
	}
	var changes, data int64
	if err := q.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&changes); err != nil {
		return "", false
	}
	if err := q.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&data); err != nil {
		return "", false
	}
	scope := "pool"
	if u, ok := ctx.Value(txKey{}).(*unit); ok {
		scope = fmt.Sprintf("tx%d", u.seq)
	}
	return fmt.Sprintf("%s.%d.%d.%s", id, changes, data, scope), true
}
