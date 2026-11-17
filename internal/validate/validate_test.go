package validate

import "testing"

func TestOperationAllowlist(t *testing.T) {
	if err := Operation("pkg-install", []string{"apt", "nginx"}); err != nil {
		t.Fatal(err)
	}
	if err := Operation("pkg-install", []string{"apt", "bash"}); err == nil {
		t.Fatal("expected package rejection")
	}
	if err := Operation("systemctl", []string{"start", "nginx"}); err != nil {
		t.Fatal(err)
	}
	if err := Operation("systemctl", []string{"start", "ssh"}); err == nil {
		t.Fatal("expected unit rejection")
	}
	if err := Operation("node-install", []string{"lts/*"}); err != nil {
		t.Fatal(err)
	}
	if err := Operation("node-install", []string{"$(id)"}); err == nil {
		t.Fatal("expected version rejection")
	}
	if err := NPMPackage("@scope/pkg-name"); err != nil {
		t.Fatal(err)
	}
	if err := NPMPackage("-rf"); err == nil {
		t.Fatal("expected npm name rejection")
	}
}

func TestNginxContent(t *testing.T) {
	ok := "# managed-by: n-manager\nserver { listen 80; }\n"
	if err := NginxContent(ok); err != nil {
		t.Fatal(err)
	}
	if err := NginxContent("server { include /etc/passwd; }\n"); err == nil {
		t.Fatal("expected unmanaged content to fail")
	}
	bad := "# managed-by: n-manager\ninclude /etc/passwd;\n"
	if err := NginxContent(bad); err == nil {
		t.Fatal("expected include rejection")
	}
}

func TestPaths(t *testing.T) {
	if err := WebRoot("/var/www/app"); err != nil {
		t.Fatal(err)
	}
	if err := WebRoot("/var/www/../etc"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if err := CertPath("/etc/letsencrypt/live/ex/fullchain.pem"); err != nil {
		t.Fatal(err)
	}
	if err := ProxyPass("http://127.0.0.1:3000"); err != nil {
		t.Fatal(err)
	}
	if err := Domain("*.example.com"); err != nil {
		t.Fatal(err)
	}
}
