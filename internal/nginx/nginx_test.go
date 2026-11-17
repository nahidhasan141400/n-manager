package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderAndRollback(t *testing.T) {
	body, err := Render(Spec{
		Name: "app", Domains: []string{"app.example.com"}, Mode: "proxy", ProxyPass: "http://127.0.0.1:3000",
		SSLCertificate: "/etc/ssl/certs/app.pem", SSLCertificateKey: "/etc/ssl/private/app.key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "server_name app.example.com;") || !strings.Contains(body, "proxy_pass http://127.0.0.1:3000;") || !strings.Contains(body, "ssl_certificate ") {
		t.Fatalf("template:\n%s", body)
	}
	redirect, err := Render(Spec{
		Name: "app", Domains: []string{"app.example.com"}, Mode: "proxy", ProxyPass: "http://127.0.0.1:3000",
		SSLCertificate: "/etc/ssl/certs/app.pem", SSLCertificateKey: "/etc/ssl/private/app.key",
		Redirect: true, BodySize: "20m", Websocket: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"return 301 https://$host$request_uri;", "client_max_body_size 20m;", "proxy_set_header Upgrade $http_upgrade;", "/.well-known/acme-challenge/"} {
		if !strings.Contains(redirect, want) {
			t.Fatalf("missing %s:\n%s", want, redirect)
		}
	}
	if _, err := Render(Spec{Name: "app", Domains: []string{"bad domain"}, Mode: "proxy", ProxyPass: "http://127.0.0.1:1"}); err == nil {
		t.Fatal("expected bad domain")
	}

	root := t.TempDir()
	layout := Layout{Kind: "sites", Available: filepath.Join(root, "sites-available"), Enabled: filepath.Join(root, "sites-enabled")}
	original := "# managed-by: n-manager\nserver { listen 80; server_name old.example.com; }\n"
	if err := os.MkdirAll(layout.Available, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Available, "app.conf"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Apply(layout, "app", body, true, func() (string, error) {
		return "fail", os.ErrInvalid
	})
	if err == nil {
		t.Fatal("expected nginx test failure")
	}
	got, err := os.ReadFile(filepath.Join(layout.Available, "app.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("config was not restored:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(layout.Enabled, "app.conf")); !os.IsNotExist(err) {
		t.Fatal("symlink should have been rolled back")
	}
	if err := Apply(layout, "app", body, true, func() (string, error) { return "ok", nil }); err != nil {
		t.Fatal(err)
	}
	sites, err := List(layout)
	if err != nil || len(sites) != 1 || !sites[0].Enabled {
		t.Fatalf("%v %+v", err, sites)
	}
}

func TestUseCertificate(t *testing.T) {
	body, err := Render(Spec{Name: "app", Domains: []string{"app.example.com"}, Mode: "root", WebRoot: "/var/www/app"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := UseCertificate(body, "/etc/letsencrypt/live/app.example.com/fullchain.pem", "/etc/letsencrypt/live/app.example.com/privkey.pem")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ssl_certificate /etc/letsencrypt/live/app.example.com/fullchain.pem;") || strings.Count(got, "acme-challenge") != 1 {
		t.Fatalf("certificate was not attached once:\n%s", got)
	}
}
