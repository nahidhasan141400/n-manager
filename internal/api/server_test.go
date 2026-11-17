package api

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"n-manager/internal/config"
	"n-manager/internal/db"
	"n-manager/internal/monitor"
	"n-manager/internal/priv"
)

func testServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	body := "server:\n  host: 127.0.0.1\n  port: 8080\nsecurity:\n  admin_password_hash: \"" + string(hash) + "\"\n  session_secret: 0123456789abcdef0123456789abcdef\n  session_ttl_minutes: 60\n  dry_run: true\ndatabase:\n  driver: sqlite\n  path: " + filepath.Join(dir, "app.db") + "\nmonitoring:\n  refresh_interval: 5\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(cfg.Data.Database.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	collector := monitor.NewCollector(time.Hour, "test")
	collector.Start()
	t.Cleanup(collector.Stop)
	handler := New(cfg, database, collector, priv.DryRun{}).Handler()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, string(hash)
}

func TestAuthMetricsAndGuards(t *testing.T) {
	ts, _ := testServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	res := do(t, client, ts.URL+"/api/system/metrics", "", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
	res = post(t, client, ts.URL+"/api/login", "", `{"password":"nope"}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login status %d", res.StatusCode)
	}
	res = post(t, client, ts.URL+"/api/login", "", `{"password":"correct-horse"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status %d body %s", res.StatusCode, readAll(res))
	}
	csrf := jsonField(t, res, "csrf_token")
	res = do(t, client, ts.URL+"/api/system/metrics", "", nil)
	if res.StatusCode != http.StatusOK || !strings.Contains(readAll(res), "mem_total") {
		t.Fatalf("metrics failed")
	}
	res = post(t, client, ts.URL+"/api/apps/mysql/service", "", `{"action":"stop","confirm":true}`)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected csrf failure, got %d", res.StatusCode)
	}
	res = post(t, client, ts.URL+"/api/apps/mysql/service", csrf, `{"action":"stop"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected confirm failure, got %d %s", res.StatusCode, readAll(res))
	}
	res = post(t, client, ts.URL+"/api/apps/mysql/service", csrf, `{"action":"stop","confirm":true}`)
	body := readAll(res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "dry_run") {
		t.Fatalf("dry run stop: %d %s", res.StatusCode, body)
	}
	res = post(t, client, ts.URL+"/api/nginx/sites", csrf, `{"name":"bad name","domains":["example.com"],"mode":"proxy","proxy_pass":"http://127.0.0.1:3000"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid site, got %d", res.StatusCode)
	}
	res = post(t, client, ts.URL+"/api/settings/password", csrf, `{"current_password":"correct-horse","new_password":"new-password"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("password change %d %s", res.StatusCode, readAll(res))
	}
}

func post(t *testing.T, client *http.Client, url, csrf, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func do(t *testing.T, client *http.Client, url, csrf string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func readAll(res *http.Response) string {
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func jsonField(t *testing.T, res *http.Response, key string) string {
	t.Helper()
	body := readAll(res)
	needle := `"` + key + `":"`
	i := strings.Index(body, needle)
	if i < 0 {
		t.Fatalf("missing %s in %s", key, body)
	}
	rest := body[i+len(needle):]
	j := strings.Index(rest, `"`)
	return rest[:j]
}
