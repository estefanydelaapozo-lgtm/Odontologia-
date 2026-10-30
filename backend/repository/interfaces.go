package repository

import "database/sql"

// DBRepository is the persistence boundary injected into the HTTP layer.
type DBRepository interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Begin() (*sql.Tx, error)
}
