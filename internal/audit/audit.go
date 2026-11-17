package audit

import (
	"database/sql"
	"time"
)

type Entry struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
}

type Logger struct {
	db *sql.DB
}

func New(database *sql.DB) *Logger {
	return &Logger{db: database}
}

func (l *Logger) Record(actor, action, target, detail, ip string) error {
	_, err := l.db.Exec(`INSERT INTO audit_log(created_at, actor, action, target, detail, ip) VALUES(?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), actor, action, target, detail, ip)
	return err
}

func (l *Logger) Recent(limit int) ([]Entry, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := l.db.Query(`SELECT id, created_at, actor, action, target, detail, ip FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.IP); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
