package config

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

func Migrate(db *sql.DB, migrations fs.FS) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reservar conexión para migraciones: %w", err)
	}
	defer conn.Close()
	const migrationLockKey int64 = 1397318479
	var lockAcquired bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_advisory_lock($1) IS NOT NULL`, migrationLockKey).Scan(&lockAcquired); err != nil {
		return fmt.Errorf("bloquear ejecución de migraciones: %w", err)
	}
	if !lockAcquired {
		return fmt.Errorf("no se obtuvo el bloqueo de migraciones")
	}
	defer func() {
		var unlocked bool
		_ = conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockKey).Scan(&unlocked)
	}()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("crear control de migraciones: %w", err)
	}
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("leer migraciones: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		var exists bool
		if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		script, readErr := fs.ReadFile(migrations, "migrations/"+name)
		if readErr != nil {
			return readErr
		}
		tx, beginErr := conn.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		if _, err = tx.Exec(string(script)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migración %s: %w", name, err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("registrar migración %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("confirmar migración %s: %w", name, err)
		}
	}
	return nil
}
