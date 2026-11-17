package config

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLoadAndUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("server:\n  host: 127.0.0.1\n  port: 8080\nsecurity:\n  admin_password_hash: \"" + string(hash) + "\"\n  session_secret: 0123456789abcdef0123456789abcdef\n  session_ttl_minutes: 30\ndatabase:\n  driver: sqlite\n  path: " + filepath.Join(dir, "app.db") + "\nmonitoring:\n  refresh_interval: 5\n")
	if err := os.WriteFile(path, content, 0o640); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetRefreshInterval(10); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Data.Monitoring.RefreshInterval != 10 {
		t.Fatalf("interval = %d", again.Data.Monitoring.RefreshInterval)
	}
	if _, err := f.SetBind("0.0.0.0", 8080, "", ""); err == nil {
		t.Fatal("expected remote bind without tls to fail")
	}
}

func TestRejectsShortSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	content := "server:\n  host: 127.0.0.1\n  port: 8080\nsecurity:\n  admin_password_hash: " + string(hash) + "\n  session_secret: short\ndatabase:\n  path: /tmp/x.db\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected short secret to fail")
	}
}
