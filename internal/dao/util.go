package dao

import (
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	entclient "github.com/kongken/ohome/internal/dao/ent"
)

const (
	keysetColCreatedAt = "created_at"
	keysetColID        = "id"
)

// Keyset builds the shared (created_at, id) comparison predicate used for
// keyset pagination: rows strictly older than the cursor position for desc
// order, strictly newer for asc. Single-table queries only (the columns are
// unqualified); the cursor value comes from the row the client last saw.
func Keyset(asc bool, createdAt time.Time, id string) *sql.Predicate {
	before := sql.LT(keysetColCreatedAt, createdAt)
	idTie := sql.LT(keysetColID, id)
	if asc {
		before = sql.GT(keysetColCreatedAt, createdAt)
		idTie = sql.GT(keysetColID, id)
	}
	return sql.Or(before, sql.And(sql.EQ(keysetColCreatedAt, createdAt), idTie))
}

// IsUniqueViolation reports whether err is a unique-constraint failure. It
// covers ent's typed wrapper plus raw driver message strings so it also
// works for errors that skipped the ent wrapping layer.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if entclient.IsConstraintError(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "violates unique constraint") || // Postgres
		strings.Contains(msg, "UNIQUE constraint failed") || // SQLite
		strings.Contains(msg, "Error 1062") // MySQL
}

// InsertOnce runs create, treating a unique-constraint failure as success
// (false): a concurrent duplicate request raced past the existence check.
// Returns whether this call newly inserted the row.
func InsertOnce(create func() error) (bool, error) {
	err := create()
	switch {
	case err == nil:
		return true, nil
	case IsUniqueViolation(err):
		return false, nil
	default:
		return false, err
	}
}
