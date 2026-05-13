package db

import (
	"embed"
	"strings"
)

// SQL contains the query statements for the database, written in SQL files
// for ease of management for larger queries.
//
//go:embed sql/*.sql
var queries embed.FS

// Load a query from the SQL directory.
func Query(name string) (string, error) {
	if !strings.HasSuffix(name, ".sql") {
		name += ".sql"
	}

	query, err := queries.ReadFile("sql/" + name)
	if err != nil {
		return "", err
	}
	return string(query), nil
}
