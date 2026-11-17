package auth

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"n-manager/internal/db"
)

func TestLoginSessionAndRateLimit(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mgr := New(database, "0123456789abcdef0123456789abcdef", time.Hour)
	cookie, csrf, exp, err := mgr.Login("correct-horse", string(hash), "10.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if csrf == "" || exp.Before(time.Now()) {
		t.Fatal("missing session")
	}
	got, err := mgr.Session(cookie)
	if err != nil || got != csrf {
		t.Fatalf("session csrf %q err %v", got, err)
	}
	if _, err := mgr.Session(cookie + "tamper"); err != ErrUnauthorized {
		t.Fatalf("tamper err %v", err)
	}
	mgr.Logout(cookie)
	if _, err := mgr.Session(cookie); err != ErrUnauthorized {
		t.Fatal("expected logout")
	}
	for i := 0; i < 5; i++ {
		_, _, _, err := mgr.Login("nope", string(hash), "10.0.0.9", "test")
		if err != ErrBadPassword {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, _, _, err := mgr.Login("correct-horse", string(hash), "10.0.0.9", "test"); err != ErrRateLimited {
		t.Fatalf("expected rate limit, got %v", err)
	}
}
