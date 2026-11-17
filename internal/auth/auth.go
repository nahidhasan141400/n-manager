package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const CookieName = "nmanager_session"

var (
	ErrBadPassword  = errors.New("incorrect password")
	ErrRateLimited  = errors.New("too many login attempts")
	ErrUnauthorized = errors.New("unauthorized")
)

type Manager struct {
	db      *sql.DB
	secret  []byte
	ttl     time.Duration
	limiter *limiter
}

func New(database *sql.DB, secret string, ttl time.Duration) *Manager {
	return &Manager{
		db:      database,
		secret:  []byte(secret),
		ttl:     ttl,
		limiter: newLimiter(5, 10*time.Minute),
	}
}

func (m *Manager) SetTTL(ttl time.Duration) {
	if ttl > 0 {
		m.ttl = ttl
	}
}

func (m *Manager) Login(password, hash, ip, ua string) (string, string, time.Time, error) {
	if m.limiter.blocked(ip) {
		return "", "", time.Time{}, ErrRateLimited
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		m.limiter.fail(ip)
		return "", "", time.Time{}, ErrBadPassword
	}
	m.limiter.reset(ip)
	return m.create(ip, ua)
}

func (m *Manager) create(ip, ua string) (string, string, time.Time, error) {
	now := time.Now()
	_, _ = m.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	id, err := randomHex(32)
	if err != nil {
		return "", "", time.Time{}, err
	}
	csrf, err := randomHex(32)
	if err != nil {
		return "", "", time.Time{}, err
	}
	exp := now.Add(m.ttl)
	_, err = m.db.Exec(`INSERT INTO sessions(id, csrf_token, created_at, expires_at, ip, user_agent) VALUES(?, ?, ?, ?, ?, ?)`,
		id, csrf, now.Unix(), exp.Unix(), ip, ua)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return m.Sign(id), csrf, exp, nil
}

func (m *Manager) Sign(id string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(id))
	return id + "." + hex.EncodeToString(mac.Sum(nil))
}

func (m *Manager) Session(cookie string) (string, error) {
	id, ok := m.idFromCookie(cookie)
	if !ok {
		return "", ErrUnauthorized
	}
	var csrf string
	var exp int64
	err := m.db.QueryRow(`SELECT csrf_token, expires_at FROM sessions WHERE id = ?`, id).Scan(&csrf, &exp)
	if err == sql.ErrNoRows {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	if time.Now().Unix() >= exp {
		_, _ = m.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
		return "", ErrUnauthorized
	}
	return csrf, nil
}

func (m *Manager) Logout(cookie string) {
	id, ok := m.idFromCookie(cookie)
	if !ok {
		return
	}
	_, _ = m.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
}

func (m *Manager) idFromCookie(cookie string) (string, bool) {
	id, sig, ok := strings.Cut(cookie, ".")
	if !ok || len(id) != 64 {
		return "", false
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(id))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", false
	}
	return id, true
}

func HashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 200 {
		return "", fmt.Errorf("password must be between 8 and 200 characters")
	}
	sum, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(sum), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type limiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}

func (l *limiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(key)
	return len(l.hits[key]) >= l.limit
}

func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(key)
	l.hits[key] = append(l.hits[key], time.Now())
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

func (l *limiter) prune(key string) {
	cut := time.Now().Add(-l.window)
	kept := l.hits[key][:0]
	for _, ts := range l.hits[key] {
		if ts.After(cut) {
			kept = append(kept, ts)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = kept
}
