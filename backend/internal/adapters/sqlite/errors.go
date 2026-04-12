package sqlite

import "strings"

// isUniqueViolation checks if the error is a SQLite UNIQUE constraint failure.
// Pass a column like "ingredients.name" to match a specific constraint, or ""
// to match any unique violation.
func isUniqueViolation(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return false
	}
	return column == "" || strings.Contains(msg, column)
}

func isForeignKeyViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
