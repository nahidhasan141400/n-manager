package priv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"n-manager/internal/firewall"
	"n-manager/internal/ftp"
	"n-manager/internal/mysqladm"
	"n-manager/internal/nginx"
	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

func nvmScript(dir, home string) string {
	return `#!/bin/bash
set -eo pipefail
export NVM_DIR=` + bashQuote(dir) + `
export HOME=` + bashQuote(home) + `
cd "$HOME"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh"
action="$1"
arg="${2:-}"
script="${3:-}"
case "$action" in
  install)
    case "$arg" in
      lts|lts/*) nvm install --lts ;;
      *) nvm install "$arg" ;;
    esac
    ver=$(nvm version)
    case "$ver" in
      v*) nvm alias default "$ver" ;;
      *) echo "node was not installed" >&2; exit 1 ;;
    esac
    ;;
  uninstall)
    nvm deactivate >/dev/null 2>&1 || true
    if [ -f "$NVM_DIR/alias/default" ]; then
      current=$(tr -d '[:space:]' < "$NVM_DIR/alias/default")
      case "$current" in
        "$arg"|"${arg#v}"|"v${arg#v}") rm -f "$NVM_DIR/alias/default" ;;
      esac
    fi
    nvm uninstall "$arg"
    ;;
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
}

func bashQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

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
	if err := requireRoot(); err != nil {
		return "", err
	}
	return Perform(ctx, op, args, stdin)
}

// Perform runs a closed operation on the machine stored in ctx.
func Perform(ctx context.Context, op string, args []string, stdin string) (string, error) {
	if err := validate.Operation(op, args); err != nil {
		return "", err
	}
	if err := validateStdin(op, stdin); err != nil {
		return "", err
	}
	switch op {
	case "pkg-install":
		return "", pkgInstall(ctx, args[0], args[1])
	case "pkg-remove":
		return "", pkgRemove(ctx, args[0], args[1])
	case "systemctl":
		return combined(ctx, "systemctl", args[0], args[1])
	case "journal":
		return combined(ctx, "journalctl", "-u", args[0], "-n", args[1], "--no-pager", "-o", "short-iso")
	case "read-file":
		b, err := remote.ReadFile(ctx, args[0])
		if err != nil {
			return "", err
		}
		if len(b) > 256*1024 {
			b = b[len(b)-256*1024:]
		}
		return string(b), nil
	case "nginx-apply":
		return nginxApply(ctx, stdin)
	case "nginx-enable":
		return nginxToggle(ctx, args[0], true)
	case "nginx-disable":
		return nginxToggle(ctx, args[0], false)
	case "nginx-delete":
		return nginxDelete(ctx, args[0])
	case "nginx-cert-put":
		return "", nginxCertPut(ctx, args[0], stdin)
	case "nginx-cert-delete":
		return "", nginxCertDelete(ctx, args[0])
	case "certbot-issue":
		return "", certbotIssue(ctx, args[0], args[1:])
	case "certbot-renew":
		return "", certbotRenew(ctx)
	case "db-bind-local":
		return "", bindLocal(ctx, args[0])
	case "mysql-list":
		kind := "nginx"
		if len(args) == 1 {
			kind = args[0]
		}
		return mysqladm.CollectJSON(ctx, kind)
	case "mysql-settings":
		connections, wait, err := validate.MySQLSettings(args[0], args[1])
		if err != nil {
			return "", err
		}
		return "", mysqladm.ApplySettings(ctx, connections, wait)
	case "mysql-db-create":
		return "", mysqladm.CreateDatabase(ctx, args[0])
	case "mysql-db-drop":
		return "", mysqladm.DropDatabase(ctx, args[0])
	case "mysql-db-export":
		return mysqladm.ExportDatabase(ctx, args[0])
	case "mysql-db-import":
		return "", mysqladm.ImportDatabase(ctx, args[0], stdin)
	case "mysql-user-create":
		return "", mysqladm.CreateUser(ctx, args[0], args[1], stdin)
	case "mysql-user-drop":
		return "", mysqladm.DropUser(ctx, args[0], args[1])
	case "mysql-user-password":
		return "", mysqladm.SetPassword(ctx, args[0], args[1], stdin)
	case "mysql-grant":
		return "", mysqladm.GrantPrivileges(ctx, args[0], args[1], args[2], args[3:])
	case "mysql-revoke":
		return "", mysqladm.Revoke(ctx, args[0], args[1], args[2])
	case "firewall-status":
		return firewall.CollectJSON(ctx)
	case "firewall-enable":
		port, _ := strconv.Atoi(args[0])
		return "", firewall.Enable(ctx, port)
	case "firewall-disable":
		return "", firewall.Disable(ctx)
	case "firewall-allow":
		port, _ := strconv.Atoi(args[2])
		sshPort, _ := strconv.Atoi(args[6])
		comment := args[5]
		if comment == "-" {
			comment = ""
		}
		return "", firewall.AddRule(ctx, args[0], args[1], port, args[3], args[4], comment, sshPort)
	case "firewall-config":
		sshPort, _ := strconv.Atoi(args[4])
		return "", firewall.ApplyConfig(ctx, args[0], args[1], args[2], args[3] == "yes", sshPort)
	case "firewall-delete":
		number, _ := strconv.Atoi(args[0])
		sshPort, _ := strconv.Atoi(args[1])
		return "", firewall.Delete(ctx, number, sshPort)
	case "firewall-update":
		number, _ := strconv.Atoi(args[0])
		port, _ := strconv.Atoi(args[3])
		sshPort, _ := strconv.Atoi(args[7])
		comment := args[6]
		if comment == "-" {
			comment = ""
		}
		return "", firewall.Update(ctx, number, args[1], args[2], port, args[4], args[5], comment, sshPort)
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
		return "", ftpApply(ctx, stdin)
	case "ftp-user-add":
		return "", ftpUserAdd(ctx, args[0], args[1], stdin)
	case "ftp-user-delete":
		return "", ftpUserDelete(ctx, args[0])
	case "ftp-user-password":
		return "", ftpUserPassword(ctx, args[0], stdin)
	case "ftp-user-lock":
		return "", ftpUserLock(ctx, args[0], args[1])
	case "ftp-user-write":
		return "", ftpUserWrite(ctx, args[0], args[1] == "yes")
	case "git-config":
		if _, err := remote.Run(ctx, "git", []string{"config", "--global", "user.name", args[0]}, ""); err != nil {
			return "", err
		}
		return remote.Run(ctx, "git", []string{"config", "--global", "user.email", args[1]}, "")
	case "git-clone":
		return remote.Run(ctx, "git", []string{"clone", "--", args[0], args[1]}, "")
	case "git-pull":
		return remote.Run(ctx, "git", []string{"-C", args[0], "pull", "--ff-only"}, "")
	case "git-github":
		return connectGitHub(ctx)
	case "git-github-test":
		return testGitHub(ctx)
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
	switch mgr {
	case "apt":
		_, err := remote.Run(ctx, "env", []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", pkg}, "")
		return err
	case "dnf":
		_, err := remote.Run(ctx, "dnf", []string{"install", "-y", pkg}, "")
		return err
	default:
		return fmt.Errorf("unsupported package manager")
	}
}

func pkgRemove(ctx context.Context, mgr, pkg string) error {
	switch mgr {
	case "apt":
		_, err := remote.Run(ctx, "env", []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "remove", "-y", pkg}, "")
		return err
	case "dnf":
		_, err := remote.Run(ctx, "dnf", []string{"remove", "-y", pkg}, "")
		return err
	default:
		return fmt.Errorf("unsupported package manager")
	}
}

func combined(ctx context.Context, name string, args ...string) (string, error) {
	return remote.Run(ctx, name, args, "")
}

func nginxTest(ctx context.Context) func() (string, error) {
	return func() (string, error) { return nginx.LiveTest(ctx) }
}

func nginxApply(ctx context.Context, stdin string) (string, error) {
	spec, err := validate.NginxApply(stdin)
	if err != nil {
		return "", err
	}
	if out, err := nginx.Syntax(ctx, spec.Content); err != nil {
		if out == "" {
			return "", err
		}
		return out, err
	}
	layout, err := nginx.Detect(ctx, "")
	if err != nil {
		return "", err
	}
	if err := nginx.Apply(ctx, layout, spec.Name, spec.Content, spec.Enabled, nginxTest(ctx)); err != nil {
		return "", err
	}
	out, rerr := combined(ctx, "systemctl", "reload", "nginx")
	if rerr != nil && !strings.Contains(rerr.Error(), "not running") && !strings.Contains(strings.ToLower(rerr.Error()), "inactive") && !strings.Contains(rerr.Error(), "not found") {
		return out, rerr
	}
	return "nginx configuration applied", nil
}

func nginxToggle(ctx context.Context, name string, enabled bool) (string, error) {
	layout, err := nginx.Detect(ctx, "")
	if err != nil {
		return "", err
	}
	site, err := nginx.Read(ctx, layout, name)
	if err != nil {
		return "", err
	}
	if out, err := nginx.Syntax(ctx, site.Config); err != nil {
		return out, err
	}
	if err := nginx.Apply(ctx, layout, name, site.Config, enabled, nginxTest(ctx)); err != nil {
		return "", err
	}
	_, _ = combined(ctx, "systemctl", "reload", "nginx")
	return "updated", nil
}

func nginxCertPut(ctx context.Context, name, stdin string) error {
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
	if err := remote.MkdirAll(ctx, nginx.CertDir, 0o755); err != nil {
		return err
	}
	certPath := filepath.Join(nginx.CertDir, name+".crt")
	keyPath := filepath.Join(nginx.CertDir, name+".key")
	if err := remote.WriteFile(ctx, certPath, []byte(body.Certificate), 0o644); err != nil {
		return err
	}
	if err := remote.WriteFile(ctx, keyPath, []byte(body.Key), 0o600); err != nil {
		return err
	}
	return nil
}

func nginxCertDelete(ctx context.Context, name string) error {
	if err := validate.SiteName(name); err != nil {
		return err
	}
	for _, path := range []string{
		filepath.Join(nginx.CertDir, name+".crt"),
		filepath.Join(nginx.CertDir, name+".key"),
	} {
		info, err := remote.Stat(ctx, path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("certificate file is not a regular file")
		}
		if err := remote.Remove(ctx, path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func certbotIssue(ctx context.Context, email string, domains []string) error {
	if err := validate.Email(email); err != nil {
		return err
	}
	if err := validate.CertbotDomains(domains); err != nil {
		return err
	}
	if err := remote.MkdirAll(ctx, "/var/www/letsencrypt", 0o755); err != nil {
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
	_, err := remote.Run(ctx, "certbot", args, "")
	return err
}

func certbotRenew(ctx context.Context) error {
	_, err := remote.Run(ctx, "certbot", []string{"renew", "--quiet", "--no-random-sleep-on-renew"}, "")
	return err
}

func nginxDelete(ctx context.Context, name string) (string, error) {
	layout, err := nginx.Detect(ctx, "")
	if err != nil {
		return "", err
	}
	if err := nginx.Delete(ctx, layout, name, nginxTest(ctx)); err != nil {
		return "", err
	}
	_, _ = combined(ctx, "systemctl", "reload", "nginx")
	return "deleted", nil
}

func bindLocal(ctx context.Context, engine string) error {
	switch engine {
	case "mysql":
		dirs := []string{"/etc/mysql/mysql.conf.d", "/etc/mysql/mariadb.conf.d", "/etc/my.cnf.d"}
		var dir string
		for _, candidate := range dirs {
			if st, err := remote.Stat(ctx, candidate); err == nil && st.IsDir() {
				dir = candidate
				break
			}
		}
		if dir == "" {
			return fmt.Errorf("mysql configuration directory was not found")
		}
		body := "# managed-by: n-manager\n[mysqld]\nbind-address = 127.0.0.1\nmysqlx-bind-address = 127.0.0.1\n"
		return remote.WriteFile(ctx, filepath.Join(dir, "zz-n-manager.cnf"), []byte(body), 0o644)
	case "postgres":
		matches, _ := remote.Glob(ctx, "/etc/postgresql/*/*/conf.d")
		if len(matches) == 0 {
			return fmt.Errorf("postgresql conf.d was not found")
		}
		body := "# managed-by: n-manager\nlisten_addresses = 'localhost'\n"
		var first error
		for _, dir := range matches {
			if err := remote.WriteFile(ctx, filepath.Join(dir, "zz-n-manager.conf"), []byte(body), 0o644); err != nil && first == nil {
				first = err
			}
		}
		return first
	default:
		return fmt.Errorf("unknown database")
	}
}

func installNVM(ctx context.Context) error {
	dir, home, err := nvmPaths(ctx)
	if err != nil {
		return err
	}
	if _, err := remote.Stat(ctx, filepath.Join(dir, "nvm.sh")); err == nil {
		return nil
	}
	if _, err := remote.Run(ctx, "git", []string{"--version"}, ""); err != nil {
		if _, aptErr := remote.Run(ctx, "env", []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "git"}, ""); aptErr != nil {
			if _, dnfErr := remote.Run(ctx, "dnf", []string{"install", "-y", "git"}, ""); dnfErr != nil {
				return fmt.Errorf("git is required to install nvm")
			}
		}
	}
	if err := remote.MkdirAll(ctx, home, 0o750); err != nil {
		return err
	}
	if _, err := remote.Stat(ctx, dir); err == nil {
		_, _ = remote.Run(ctx, "rm", []string{"-rf", "--", dir}, "")
	}
	if _, err := remote.Run(ctx, "git", []string{"clone", "--depth", "1", "--branch", "v0.40.3", "https://github.com/nvm-sh/nvm.git", dir}, ""); err != nil {
		return err
	}
	if _, local := remote.From(ctx).(remote.Local); local {
		_, _ = remote.Run(ctx, "chown", []string{"-R", "nmanager-deploy:nmanager-deploy", dir, home}, "")
		return nil
	}
	return ensureNVMProfile(ctx, home)
}

func nvmPaths(ctx context.Context) (dir, home string, err error) {
	if _, ok := remote.From(ctx).(remote.Local); ok {
		return "/var/lib/n-manager/nvm", "/var/lib/n-manager/deploy", nil
	}
	out, runErr := remote.Run(ctx, "bash", []string{"-c", `printf %s "$HOME"`}, "")
	home = strings.TrimSpace(out)
	if runErr != nil || !strings.HasPrefix(home, "/") || strings.Contains(home, "..") || strings.ContainsAny(home, "\n\r\x00") {
		return "", "", fmt.Errorf("the server home directory could not be read")
	}
	return filepath.Join(home, ".nvm"), home, nil
}

func ensureNVMProfile(ctx context.Context, home string) error {
	const marker = "managed-by: n-manager"
	snippet := "\n# " + marker + "\nexport NVM_DIR=\"$HOME/.nvm\"\n[ -s \"$NVM_DIR/nvm.sh\" ] && . \"$NVM_DIR/nvm.sh\"\n[ -s \"$NVM_DIR/bash_completion\" ] && . \"$NVM_DIR/bash_completion\"\n"
	path := filepath.Join(home, ".bashrc")
	current, err := remote.ReadFile(ctx, path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(current), marker) || strings.Contains(string(current), "NVM_DIR") {
		return nil
	}
	return remote.WriteFile(ctx, path, append(current, []byte(snippet)...), 0o644)
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
	dir, home, err := nvmPaths(ctx)
	if err != nil {
		return err
	}
	scriptPath := "/var/lib/n-manager/nvm-run.sh"
	if err := remote.WriteFile(ctx, scriptPath, []byte(nvmScript(dir, home)), 0o755); err != nil {
		return err
	}
	argv := []string{scriptPath, action, arg}
	if len(extra) > 0 && extra[0] != "" {
		argv = append(argv, extra[0])
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if _, ok := remote.From(ctx).(remote.Local); ok {
		runArgs := append([]string{"-u", "nmanager-deploy", "--", "/bin/bash"}, argv...)
		_, err := remote.Run(ctx, "runuser", runArgs, "")
		if err != nil {
			return err
		}
	} else if _, err = remote.Run(ctx, "/bin/bash", argv, ""); err != nil {
		return err
	}
	switch action {
	case "install", "default":
		return activateNode(ctx, dir)
	case "uninstall":
		return clearNodeLinks(ctx)
	default:
		return nil
	}
}

func clearNodeLinks(ctx context.Context) error {
	for _, name := range []string{"node", "npm", "npx"} {
		link := "/usr/local/bin/" + name
		target, err := remote.Readlink(ctx, link)
		if err != nil || !strings.Contains(target, "/.nvm/") {
			continue
		}
		if err := remote.Remove(ctx, link); err != nil {
			return err
		}
	}
	return nil
}

func activateNode(ctx context.Context, nvmDir string) error {
	raw, err := remote.ReadFile(ctx, filepath.Join(nvmDir, "alias", "default"))
	if err != nil {
		return nil
	}
	version := strings.TrimSpace(string(raw))
	if version == "" {
		return nil
	}
	if !strings.HasPrefix(version, "v") && numericVersion(version) {
		version = "v" + version
	}
	binDir := filepath.Join(nvmDir, "versions", "node", version, "bin")
	node := filepath.Join(binDir, "node")
	if _, err := remote.Stat(ctx, node); err != nil {
		alias, readErr := remote.ReadFile(ctx, filepath.Join(nvmDir, "alias", version))
		next := strings.TrimSpace(string(alias))
		if readErr != nil || !strings.HasPrefix(next, "v") {
			return fmt.Errorf("node was not installed")
		}
		version = next
		binDir = filepath.Join(nvmDir, "versions", "node", version, "bin")
		node = filepath.Join(binDir, "node")
		if _, err := remote.Stat(ctx, node); err != nil {
			return fmt.Errorf("node was not installed")
		}
	}
	if _, err := remote.Run(ctx, node, []string{"-v"}, ""); err != nil && strings.Contains(err.Error(), "libatomic") {
		if err := installLibatomic(ctx); err != nil {
			return err
		}
		if _, err := remote.Run(ctx, node, []string{"-v"}, ""); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for _, name := range []string{"node", "npm", "npx"} {
		target := filepath.Join(binDir, name)
		if _, err := remote.Stat(ctx, target); err != nil {
			continue
		}
		if _, err := remote.Run(ctx, "ln", []string{"-sfn", "--", target, "/usr/local/bin/" + name}, ""); err != nil {
			return err
		}
	}
	return nil
}

func numericVersion(value string) bool {
	if value == "" {
		return false
	}
	dots := 0
	for _, ch := range value {
		if ch == '.' {
			dots++
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return dots <= 2
}

func installLibatomic(ctx context.Context) error {
	if _, err := remote.Run(ctx, "env", []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "libatomic1"}, ""); err == nil {
		return nil
	}
	if _, err := remote.Run(ctx, "dnf", []string{"install", "-y", "libatomic"}, ""); err != nil {
		return fmt.Errorf("libatomic is required to run node")
	}
	return nil
}

func ftpApply(ctx context.Context, stdin string) error {
	spec, err := validate.FTPApplyPayload(stdin)
	if err != nil {
		return err
	}
	if _, err := remote.Stat(ctx, ftp.ConfPath); err == nil {
		backup := ftp.ConfPath + ".n-manager.bak"
		if _, err := remote.Stat(ctx, backup); os.IsNotExist(err) {
			src, rerr := remote.ReadFile(ctx, ftp.ConfPath)
			if rerr == nil {
				_ = remote.WriteFile(ctx, backup, src, 0o644)
			}
		}
	}
	if err := remote.WriteFile(ctx, ftp.ConfPath, []byte(ftp.Render(spec)), 0o644); err != nil {
		return err
	}
	if _, err := remote.Stat(ctx, ftp.UserList); os.IsNotExist(err) {
		if err := remote.WriteFile(ctx, ftp.UserList, []byte("# managed-by: n-manager\n"), 0o644); err != nil {
			return err
		}
	}
	return remote.MkdirAll(ctx, ftp.UserConfDir, 0o755)
}

func ftpUserAdd(ctx context.Context, name, home, password string) error {
	if err := validate.FTPUser(name, home); err != nil {
		return err
	}
	if err := validate.FTPPassword(password); err != nil {
		return err
	}
	if err := remote.MkdirAll(ctx, filepath.Dir(home), 0o755); err != nil {
		return err
	}
	if _, err := remote.Run(ctx, "id", []string{name}, ""); err == nil {
		if !ftpListed(ctx, name) {
			return fmt.Errorf("user already exists")
		}
	} else {
		if _, err := remote.Run(ctx, "useradd", []string{"--create-home", "--home-dir", home, "--shell", ftp.NologinShell(ctx), "--comment", "managed-by: n-manager", name}, ""); err != nil {
			return err
		}
	}
	if err := chpasswd(ctx, name, password); err != nil {
		return err
	}
	if err := ftpEnsureListed(ctx, name); err != nil {
		return err
	}
	return ftpUserWrite(ctx, name, true)
}

func ftpUserPassword(ctx context.Context, name, password string) error {
	if err := validate.FTPName(name); err != nil {
		return err
	}
	if err := validate.FTPPassword(password); err != nil {
		return err
	}
	if !ftpListed(ctx, name) {
		return fmt.Errorf("ftp user was not found")
	}
	return chpasswd(ctx, name, password)
}

func ftpUserLock(ctx context.Context, name, mode string) error {
	if err := validate.FTPName(name); err != nil {
		return err
	}
	if mode != "lock" && mode != "unlock" {
		return fmt.Errorf("invalid ftp user request")
	}
	if !ftpListed(ctx, name) {
		return fmt.Errorf("ftp user was not found")
	}
	flag := "--lock"
	if mode == "unlock" {
		flag = "--unlock"
	}
	_, err := remote.Run(ctx, "usermod", []string{flag, name}, "")
	return err
}

func ftpUserWrite(ctx context.Context, name string, write bool) error {
	path, err := ftp.UserConfPath(name)
	if err != nil {
		return err
	}
	if err := remote.MkdirAll(ctx, ftp.UserConfDir, 0o755); err != nil {
		return err
	}
	return remote.WriteFile(ctx, path, []byte(ftp.RenderUser(write)), 0o644)
}

func ftpUserDelete(ctx context.Context, name string) error {
	if err := validate.FTPName(name); err != nil {
		return err
	}
	if !ftpListed(ctx, name) {
		return fmt.Errorf("ftp user was not found")
	}
	if _, err := remote.Run(ctx, "id", []string{name}, ""); err == nil {
		if _, err := remote.Run(ctx, "userdel", []string{name}, ""); err != nil {
			return err
		}
	}
	if err := ftpUnlist(ctx, name); err != nil {
		return err
	}
	path, err := ftp.UserConfPath(name)
	if err != nil {
		return err
	}
	if err := remote.Remove(ctx, path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func chpasswd(ctx context.Context, name, password string) error {
	_, err := remote.Run(ctx, "chpasswd", nil, name+":"+password+"\n")
	return err
}

func ftpListed(ctx context.Context, name string) bool {
	b, err := remote.ReadFile(ctx, ftp.UserList)
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

func ftpEnsureListed(ctx context.Context, name string) error {
	if ftpListed(ctx, name) {
		return nil
	}
	b, err := remote.ReadFile(ctx, ftp.UserList)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	body := string(b)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += name + "\n"
	return remote.WriteFile(ctx, ftp.UserList, []byte(body), 0o644)
}

func ftpUnlist(ctx context.Context, name string) error {
	b, err := remote.ReadFile(ctx, ftp.UserList)
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
	return remote.WriteFile(ctx, ftp.UserList, []byte(body), 0o644)
}
