package config

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func LoadEnv() { _ = godotenv.Load() }

func Getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func ConnectDB() (*sql.DB, error) {
	databaseURL := Getenv("DATABASE_URL", "")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL es obligatorio; configúralo en backend/.env")
	}
	conn, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("abrir PostgreSQL: %w", err)
	}
	maxOpen, err := strconv.Atoi(Getenv("DB_MAX_OPEN_CONNS", "150"))
	if err != nil || maxOpen < 1 {
		_ = conn.Close()
		return nil, fmt.Errorf("DB_MAX_OPEN_CONNS debe ser un entero positivo")
	}
	maxIdle, err := strconv.Atoi(Getenv("DB_MAX_IDLE_CONNS", "30"))
	if err != nil || maxIdle < 0 || maxIdle > maxOpen {
		_ = conn.Close()
		return nil, fmt.Errorf("DB_MAX_IDLE_CONNS debe ser un entero entre 0 y DB_MAX_OPEN_CONNS")
	}
	conn.SetMaxOpenConns(maxOpen)
	conn.SetMaxIdleConns(maxIdle)
	conn.SetConnMaxLifetime(30 * time.Minute)
	if err = conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	return conn, nil
}
