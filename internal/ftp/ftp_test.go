package ftp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"n-manager/internal/validate"
)

func TestRenderDisablesAnonymous(t *testing.T) {
	spec := validate.FTPDefaults()
	spec.PasvAddress = "203.0.113.10"
	spec.Banner = "N Manager FTP"
	spec.MaxRate = 1024
	body := Render(spec)
	for _, want := range []string{
		"anonymous_enable=NO", "chroot_local_user=YES", "listen_port=21",
		"pasv_address=203.0.113.10", "write_enable=YES", "ssl_enable=NO",
		"local_umask=022", "max_clients=50", "local_max_rate=1024",
		"ftpd_banner=N Manager FTP", "xferlog_enable=YES", "# managed-by: n-manager",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("config missing %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "anonymous_enable=YES") {
		t.Fatal("anonymous access was enabled")
	}
}

func TestParseConfRoundTrip(t *testing.T) {
	spec := validate.FTPDefaults()
	spec.TLS = true
	spec.TLSCert = "/etc/ssl/certs/ftp.pem"
	spec.TLSKey = "/etc/ssl/private/ftp.key"
	spec.Write = false
	dir := t.TempDir()
	path := filepath.Join(dir, "vsftpd.conf")
	os.WriteFile(path, []byte(Render(spec)), 0o644)
	got, err := parseConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Write || !got.TLS || got.TLSCert != spec.TLSCert || got.Idle != 600 || got.Umask != "022" {
		t.Fatalf("parsed %#v", got)
	}
}

func TestListUsers(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "userlist")
	passwd := filepath.Join(dir, "passwd")
	shadow := filepath.Join(dir, "shadow")
	os.WriteFile(list, []byte("# comment\nftpdemo\n"), 0o644)
	os.WriteFile(passwd, []byte("ftpdemo:x:1001:1001::/srv/ftp/ftpdemo:/usr/sbin/nologin\n"), 0o644)
	os.WriteFile(shadow, []byte("ftpdemo:!$6$hash:19000:0:99999:7:::\n"), 0o644)
	users := listUsers(list, passwd, shadow, true)
	if len(users) != 1 || users[0].Name != "ftpdemo" || users[0].Home != "/srv/ftp/ftpdemo" || users[0].Locked == nil || !*users[0].Locked {
		t.Fatalf("users %#v", users)
	}
}

func TestListHomesSkipsHiddenSymlinksAndHeavyDirs(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "projects", "site"), 0o755)
	os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755)
	os.MkdirAll(filepath.Join(root, ".hidden", "secret"), 0o755)
	os.Symlink(filepath.Join(root, "projects"), filepath.Join(root, "link"))
	got := ListHomes([]string{root}, 4, 20)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, filepath.Join(root, "projects", "site")) {
		t.Fatalf("missing nested directory: %#v", got)
	}
	for _, path := range got {
		base := filepath.Base(path)
		if base == "node_modules" || base == "pkg" || base == ".hidden" || base == "secret" || base == "link" || path == root {
			t.Fatalf("unexpected path %s in %#v", path, got)
		}
	}
}

func TestParseSessions(t *testing.T) {
	text := "0 0 127.0.0.1:80 10.0.0.2:1\n0 0 192.168.0.8:21 10.1.1.9:54000\n0 0 [::1]:21 [::2]:9\n"
	got := ParseSessions(text, 21)
	if len(got) != 2 || got[0].Peer != "10.1.1.9:54000" || got[1].Local != "[::1]:21" {
		t.Fatalf("sessions %#v", got)
	}
}
