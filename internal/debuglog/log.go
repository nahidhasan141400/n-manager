package debuglog

import (
	"database/sql"
	"strings"
	"sync"
	"time"
)

const keep = 400

// Entry is one debug line.
type Entry struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Level     string `json:"level"`
	Server    string `json:"server"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Detail    string `json:"detail"`
}

// Log stores a short ring of debug lines in SQLite.
type Log struct {
	mu     sync.Mutex
	db     *sql.DB
	last   string
	lastAt time.Time
}

func New(database *sql.DB) *Log {
	return &Log{db: database}
}

// Note records one line. An identical line is kept once per 30 seconds.
func (l *Log) Note(level, server, kind, message, detail string) {
	if l == nil || l.db == nil {
		return
	}
	if level != "error" {
		level = "info"
	}
	message = clip(strings.TrimSpace(message), 500)
	detail = clip(strings.TrimSpace(detail), 2000)
	server = clip(strings.TrimSpace(server), 80)
	kind = clip(strings.TrimSpace(kind), 40)
	if message == "" {
		message = "event"
	}
	key := level + "\n" + server + "\n" + kind + "\n" + message + "\n" + detail
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if key == l.last && now.Sub(l.lastAt) < 30*time.Second {
		return
	}
	if _, err := l.db.Exec(`INSERT INTO debug_log(created_at, level, server, kind, message, detail) VALUES(?, ?, ?, ?, ?, ?)`,
		now.Unix(), level, server, kind, message, detail); err != nil {
		return
	}
	_, _ = l.db.Exec(`DELETE FROM debug_log WHERE id <= COALESCE((SELECT id FROM debug_log ORDER BY id DESC LIMIT 1 OFFSET ?), 0)`, keep)
	l.last = key
	l.lastAt = now
}

func (l *Log) Recent(limit int, level string) ([]Entry, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > keep {
		limit = keep
	}
	level = strings.TrimSpace(level)
	query := `SELECT id, created_at, level, server, kind, message, detail FROM debug_log`
	args := []any{}
	if level == "error" || level == "info" {
		query += ` WHERE level = ?`
		args = append(args, level)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Level, &e.Server, &e.Kind, &e.Message, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (l *Log) Clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = ""
	_, err := l.db.Exec(`DELETE FROM debug_log`)
	return err
}

func clip(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n]
}
