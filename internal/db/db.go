package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const LatestVersion = 3

var migrations = []string{
	`CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		csrf_token TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at INTEGER NOT NULL,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
	`CREATE TABLE servers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL,
		username TEXT NOT NULL,
		password BLOB NOT NULL,
		host_key BLOB NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`,
	`CREATE TABLE debug_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at INTEGER NOT NULL,
		level TEXT NOT NULL,
		server TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL,
		message TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT ''
	);`,
}

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(4)
	database.SetConnMaxLifetime(0)
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func migrate(database *sql.DB) error {
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return err
	}
	for i, stmt := range migrations {
		version := i + 1
		var found int
		err := database.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&found)
		if err != nil {
			return err
		}
		if found > 0 {
			continue
		}
		tx, err := database.Begin()
		if err != nil {
			return err
		}
		if err := execScript(tx, stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, version, time.Now().Unix()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func execScript(tx *sql.Tx, script string) error {
	for _, stmt := range strings.Split(script, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func CurrentVersion(database *sql.DB) (int, error) {
	var version int
	err := database.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	return version, err
}

type Status struct {
	Driver  string `json:"driver"`
	Path    string `json:"path"`
	Open    bool   `json:"open"`
	Version int    `json:"version"`
	Latest  int    `json:"latest"`
	Ready   bool   `json:"ready"`
}

func StatusOf(database *sql.DB, path string) (Status, error) {
	st := Status{Driver: "sqlite", Path: path, Latest: LatestVersion}
	if err := database.Ping(); err != nil {
		return st, err
	}
	st.Open = true
	version, err := CurrentVersion(database)
	if err != nil {
		return st, err
	}
	st.Version = version
	st.Ready = version == LatestVersion
	return st, nil
}
