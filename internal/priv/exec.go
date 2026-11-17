package priv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"os/user"

	"n-manager/internal/ftp"
	"n-manager/internal/mysqladm"
	"n-manager/internal/nginx"
	"n-manager/internal/validate"
)

const nvmScript = `#!/bin/bash
set -euo pipefail
export NVM_DIR=/var/lib/n-manager/nvm
export HOME=/var/lib/n-manager/deploy
cd "$HOME"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh"
action="$1"
arg="${2:-}"
script="${3:-}"
case "$action" in
  install) nvm install "$arg" ;;
  uninstall) nvm uninstall "$arg" ;;
  default) nvm alias default "$arg" ;;
  npm-install) npm install -g -- "$arg" ;;
  npm-uninstall) npm uninstall -g -- "$arg" ;;
  pm2-install) npm install -g pm2 ;;
  pm2-stop) pm2 stop "$arg" ;;
  pm2-restart) pm2 restart "$arg" ;;
  pm2-delete) pm2 delete "$arg" ;;
  pm2-logs) pm2 logs "$arg" --nostream --lines 100 --raw ;;
  pm2-start)
    if [ -n "$script" ]; then
      pm2 start "$script" --name "$arg"
    else
      pm2 start "$arg"
    fi
    ;;
  *) echo "unknown action" >&2; exit 2 ;;
esac
`

func Main(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: n-manager priv <operation> [args]")
		return 2
	}
	stdin, _ := io.ReadAll(os.Stdin)
	out, err := Execute(context.Background(), args[0], args[1:], string(stdin))
	if out != "" {
		fmt.Println(out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func Execute(ctx context.Context, op string, args []string, stdin string) (string, error) {
	if err := validate.Operation(op, args); err != nil {
		return "", err
	}
	if err := requireRoot(); err != nil {
		return "", err
	}
	switch op {
	case "pkg-install":
		return "", pkgInstall(ctx, args[0], args[1])
	case "systemctl":
		return combined(ctx, "systemctl", args[0], args[1])
	case "journal":
		return combined(ctx, "journalctl", "-u", args[0], "-n", args[1], "--no-pager", "-o", "short-iso")
	case "read-file":
		b, err := os.ReadFile(args[0])
		if err != nil {
			return "", err
		}
		if len(b) > 256*1024 {
			b = b[len(b)-256*1024:]
		}
		return string(b), nil
	case "nginx-apply":
		return nginxApply(stdin)
	case "nginx-enable":
		return nginxToggle(args[0], true)
	case "nginx-disable":
		return nginxToggle(args[0], false)
	case "nginx-delete":
		return nginxDelete(args[0])
	case "nginx-cert-put":
		return "", nginxCertPut(args[0], stdin)
	case "nginx-cert-delete":
		return "", nginxCertDelete(args[0])
	case "certbot-issue":
		return "", certbotIssue(args[0], args[1:])
	case "certbot-renew":
		return "", certbotRenew()
	case "db-bind-local":
		return "", bindLocal(args[0])
	case "mysql-list":
		return mysqladm.CollectJSON()
	case "mysql-settings":
		connections, wait, err := validate.MySQLSettings(args[0], args[1])
		if err != nil {
			return "", err
		}
		return "", mysqladm.ApplySettings(connections, wait)
	case "mysql-db-create":
		return "", mysqladm.CreateDatabase(args[0])
	case "mysql-db-drop":
		return "", mysqladm.DropDatabase(args[0])
	case "mysql-user-create":
		return "", mysqladm.CreateUser(args[0], args[1], stdin)
	case "mysql-user-drop":
		return "", mysqladm.DropUser(args[0], args[1])
	case "mysql-user-password":
		return "", mysqladm.SetPassword(args[0], args[1], stdin)
	case "mysql-grant":
		return "", mysqladm.GrantPrivileges(args[0], args[1], args[2], args[3:])
	case "mysql-revoke":
		return "", mysqladm.Revoke(args[0], args[1], args[2])
	case "phpmyadmin-install":
		return "", mysqladm.InstallPHPMyAdmin(ctx, args[0])
	case "phpmyadmin-enable":
		return "", mysqladm.SetPHPMyAdmin(true)
	case "phpmyadmin-disable":
		return "", mysqladm.SetPHPMyAdmin(false)
	case "nvm-install":
		return "", installNVM(ctx)
	case "node-install":
		return "", nvmAction(ctx, "install", args[0])
	case "node-uninstall":
		return "", nvmAction(ctx, "uninstall", args[0])
	case "node-default":
		return "", nvmAction(ctx, "default", args[0])
	case "npm-global-install":
		return "", nvmAction(ctx, "npm-install", args[0])
	case "npm-global-uninstall":
		return "", nvmAction(ctx, "npm-uninstall", args[0])
	case "pm2":
		return pm2Managed(ctx, args)
	case "ftp-apply":
		return "", ftpApply(stdin)
	case "ftp-user-add":
		return "", ftpUserAdd(args[0], args[1], stdin)
	case "ftp-user-delete":
		return "", ftpUserDelete(args[0])
	case "ftp-user-password":
		return "", ftpUserPassword(args[0], stdin)
	case "ftp-user-lock":
		return "", ftpUserLock(args[0], args[1])
	case "ftp-user-write":
		return "", ftpUserWrite(args[0], args[1] == "yes")
	default:
		return "", fmt.Errorf("unknown operation")
	}
}

func requireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("privileged operation must run as root")
	}
	return nil
}

func pkgInstall(ctx context.Context, mgr, pkg string) error {
	var cmd *exec.Cmd
	switch mgr {
	case "apt":
		cmd = exec.CommandContext(ctx, "apt-get", "install", "-y", pkg)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	case "dnf":
		cmd = exec.CommandContext(ctx, "dnf", "install", "-y", pkg)
	default:
		return fmt.Errorf("unsupported package manager")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func combined(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}

func nginxApply(stdin string) (string, error) {
	spec, err := validate.NginxApply(stdin)
	if err != nil {
		return "", err
	}
	if out, err := nginx.Syntax(spec.Content); err != nil {
		if out == "" {
			return "", err
		}
		return out, err
	}
	layout, err := nginx.Detect("")
	if err != nil {
		return "", err
	}
	if err := nginx.Apply(layout, spec.Name, spec.Content, spec.Enabled, nginx.LiveTest); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		out, rerr := combined(context.Background(), "systemctl", "reload", "nginx")
		if rerr != nil && !strings.Contains(rerr.Error(), "not running") && !strings.Contains(strings.ToLower(rerr.Error()), "inactive") {
			return out, rerr
		}
	}
	return "nginx configuration applied", nil
}

func nginxToggle(name string, enabled bool) (string, error) {
	layout, err := nginx.Detect("")
	if err != nil {
		return "", err
	}
	site, err := nginx.Read(layout, name)
	if err != nil {
		return "", err
	}
	if out, err := nginx.Syntax(site.Config); err != nil {
		return out, err
	}
	if err := nginx.Apply(layout, name, site.Config, enabled, nginx.LiveTest); err != nil {
		return "", err
	}
	_, _ = combined(context.Background(), "systemctl", "reload", "nginx")
	return "updated", nil
}

func nginxCertPut(name, stdin string) error {
	if err := validate.SiteName(name); err != nil {
		return err
	}
	if err := validate.NginxCertPayload(stdin); err != nil {
		return err
	}
	var body validate.NginxCert
	if err := json.Unmarshal([]byte(stdin), &body); err != nil {
		return err
	}
	if err := os.MkdirAll(nginx.CertDir, 0o755); err != nil {
		return err
	}
	certPath := filepath.Join(nginx.CertDir, name+".crt")
	keyPath := filepath.Join(nginx.CertDir, name+".key")
	if err := os.WriteFile(certPath, []byte(body.Certificate), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, []byte(body.Key), 0o600); err != nil {
		return err
	}
	return nil
}

func nginxCertDelete(name string) error {
	if err := validate.SiteName(name); err != nil {
		return err
	}
	for _, path := range []string{
		filepath.Join(nginx.CertDir, name+".crt"),
		filepath.Join(nginx.CertDir, name+".key"),
	} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("certificate file is not a regular file")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func certbotIssue(email string, domains []string) error {
	if err := validate.Email(email); err != nil {
		return err
	}
	if err := validate.CertbotDomains(domains); err != nil {
		return err
	}
	if err := os.MkdirAll("/var/www/letsencrypt", 0o755); err != nil {
		return err
	}
	args := []string{
		"certonly", "--webroot", "-w", "/var/www/letsencrypt",
		"--non-interactive", "--agree-tos", "--no-eff-email", "--keep-until-expiring",
		"--email", email,
	}
	for _, domain := range domains {
		args = append(args, "-d", domain)
	}
	out, err := exec.Command("certbot", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func certbotRenew() error {
	out, err := exec.Command("certbot", "renew", "--quiet", "--no-random-sleep-on-renew").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func nginxDelete(name string) (string, error) {
	layout, err := nginx.Detect("")
	if err != nil {
		return "", err
	}
	if err := nginx.Delete(layout, name, nginx.LiveTest); err != nil {
		return "", err
	}
	_, _ = combined(context.Background(), "systemctl", "reload", "nginx")
	return "deleted", nil
}

func bindLocal(engine string) error {
	switch engine {
	case "mysql":
		dirs := []string{"/etc/mysql/mysql.conf.d", "/etc/mysql/mariadb.conf.d", "/etc/my.cnf.d"}
		var dir string
		for _, candidate := range dirs {
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				dir = candidate
				break
			}
		}
		if dir == "" {
			return fmt.Errorf("mysql configuration directory was not found")
		}
		body := "# managed-by: n-manager\n[mysqld]\nbind-address = 127.0.0.1\nmysqlx-bind-address = 127.0.0.1\n"
		return os.WriteFile(filepath.Join(dir, "zz-n-manager.cnf"), []byte(body), 0o644)
	case "postgres":
		matches, _ := filepath.Glob("/etc/postgresql/*/*/conf.d")
		if len(matches) == 0 {
			return fmt.Errorf("postgresql conf.d was not found")
		}
		body := "# managed-by: n-manager\nlisten_addresses = 'localhost'\n"
		var first error
		for _, dir := range matches {
			if err := os.WriteFile(filepath.Join(dir, "zz-n-manager.conf"), []byte(body), 0o644); err != nil && first == nil {
				first = err
			}
		}
		return first
	default:
		return fmt.Errorf("unknown database")
	}
}

func installNVM(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join("/var/lib/n-manager/nvm", "nvm.sh")); err == nil {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required to install nvm")
	}
	if err := os.MkdirAll("/var/lib/n-manager", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll("/var/lib/n-manager/deploy", 0o750); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", "v0.40.3", "https://github.com/nvm-sh/nvm.git", "/var/lib/n-manager/nvm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	if err := os.WriteFile("/var/lib/n-manager/nvm-run.sh", []byte(nvmScript), 0o755); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", "nmanager-deploy:nmanager-deploy", "/var/lib/n-manager/nvm", "/var/lib/n-manager/deploy").Run()
	return nil
}

func pm2Managed(ctx context.Context, args []string) (string, error) {
	action := "pm2-" + args[0]
	name := ""
	script := ""
	if len(args) > 1 {
		name = args[1]
	}
	if len(args) > 2 {
		script = args[2]
	}
	if err := nvmAction(ctx, action, name, script); err != nil {
		return "", err
	}
	return "ok", nil
}

func nvmAction(ctx context.Context, action, arg string, extra ...string) error {
	scriptPath := "/var/lib/n-manager/nvm-run.sh"
	if err := os.WriteFile(scriptPath, []byte(nvmScript), 0o755); err != nil {
		return err
	}
	argv := []string{"-u", "nmanager-deploy", "--", "/bin/bash", scriptPath, action, arg}
	if len(extra) > 0 {
		argv = append(argv, extra[0])
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "runuser", argv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func ftpApply(stdin string) error {
	spec, err := validate.FTPApplyPayload(stdin)
	if err != nil {
		return err
	}
	if _, err := os.Stat(ftp.ConfPath); err == nil {
		backup := ftp.ConfPath + ".n-manager.bak"
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			src, rerr := os.ReadFile(ftp.ConfPath)
			if rerr == nil {
				_ = os.WriteFile(backup, src, 0o644)
			}
		}
	}
	if err := os.WriteFile(ftp.ConfPath, []byte(ftp.Render(spec)), 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(ftp.UserList); os.IsNotExist(err) {
		if err := os.WriteFile(ftp.UserList, []byte("# managed-by: n-manager\n"), 0o644); err != nil {
			return err
		}
	}
	return os.MkdirAll(ftp.UserConfDir, 0o755)
}

func ftpUserAdd(name, home, password string) error {
	if err := validate.FTPUser(name, home); err != nil {
		return err
	}
	if err := validate.FTPPassword(password); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(home), 0o755); err != nil {
		return err
	}
	if _, err := user.Lookup(name); err == nil {
		if !ftpListed(name) {
			return fmt.Errorf("user already exists")
		}
	} else {
		cmd := exec.Command("useradd", "--create-home", "--home-dir", home, "--shell", ftp.NologinShell(), "--comment", "managed-by: n-manager", name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				return err
			}
			return fmt.Errorf("%s", text)
		}
	}
	if err := chpasswd(name, password); err != nil {
		return err
	}
	if err := ftpEnsureListed(name); err != nil {
		return err
	}
	return ftpUserWrite(name, true)
}

func ftpUserPassword(name, password string) error {
	if err := validate.FTPName(name); err != nil {
		return err
	}
	if err := validate.FTPPassword(password); err != nil {
		return err
	}
	if !ftpListed(name) {
		return fmt.Errorf("ftp user was not found")
	}
	return chpasswd(name, password)
}

func ftpUserLock(name, mode string) error {
	if err := validate.FTPName(name); err != nil {
		return err
	}
	if mode != "lock" && mode != "unlock" {
		return fmt.Errorf("invalid ftp user request")
	}
	if !ftpListed(name) {
		return fmt.Errorf("ftp user was not found")
	}
	flag := "--lock"
	if mode == "unlock" {
		flag = "--unlock"
	}
	out, err := exec.Command("usermod", flag, name).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func ftpUserWrite(name string, write bool) error {
	path, err := ftp.UserConfPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(ftp.UserConfDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(ftp.RenderUser(write)), 0o644)
}

func ftpUserDelete(name string) error {
	if err := validate.FTPName(name); err != nil {
		return err
	}
	if !ftpListed(name) {
		return fmt.Errorf("ftp user was not found")
	}
	if _, err := user.Lookup(name); err == nil {
		out, err := exec.Command("userdel", name).CombinedOutput()
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				return err
			}
			return fmt.Errorf("%s", text)
		}
	}
	if err := ftpUnlist(name); err != nil {
		return err
	}
	path, err := ftp.UserConfPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func chpasswd(name, password string) error {
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(name + ":" + password + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func ftpListed(name string) bool {
	b, err := os.ReadFile(ftp.UserList)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func ftpEnsureListed(name string) error {
	if ftpListed(name) {
		return nil
	}
	f, err := os.OpenFile(ftp.UserList, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(name + "\n")
	return err
}

func ftpUnlist(name string) error {
	b, err := os.ReadFile(ftp.UserList)
	if err != nil {
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == name {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	body := strings.Join(kept, "\n")
	if body != "" {
		body += "\n"
	}
	return os.WriteFile(ftp.UserList, []byte(body), 0o644)
}
