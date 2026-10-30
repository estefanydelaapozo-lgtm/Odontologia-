package repository

import "database/sql"

type PostgresRepo struct{ db *sql.DB }

func NewPostgresRepo(db *sql.DB) *PostgresRepo { return &PostgresRepo{db: db} }
func (r *PostgresRepo) Exec(query string, args ...any) (sql.Result, error) {
	return r.db.Exec(query, args...)
}
func (r *PostgresRepo) Query(query string, args ...any) (*sql.Rows, error) {
	return r.db.Query(query, args...)
}
func (r *PostgresRepo) QueryRow(query string, args ...any) *sql.Row {
	return r.db.QueryRow(query, args...)
}
func (r *PostgresRepo) Begin() (*sql.Tx, error) { return r.db.Begin() }
