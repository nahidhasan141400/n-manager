package mysqladm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"n-manager/internal/nginx"
	"n-manager/internal/pkgmgr"
	"n-manager/internal/validate"
)

const phpMyAdminSite = "phpmyadmin"

var (
	phpUnitRe = regexp.MustCompile(`^php[0-9]+\.[0-9]+-fpm$`)
	phpSockRe = regexp.MustCompile(`^/run/php/php[0-9]+\.[0-9]+-fpm\.sock$`)
	systemDBs = map[string]struct{}{"mysql": {}, "information_schema": {}, "performance_schema": {}, "sys": {}}
)

type Database struct {
	Name   string `json:"name"`
	System bool   `json:"system"`
}

type User struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	System bool   `json:"system"`
}

type Grant struct {
	User       string   `json:"user"`
	Host       string   `json:"host"`
	Database   string   `json:"database"`
	Privileges []string `json:"privileges"`
	System     bool     `json:"system"`
}

type PHPMyAdmin struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Enabled   bool   `json:"enabled"`
	Mode      string `json:"mode"`
}

type Admin struct {
	Databases  []Database `json:"databases"`
	Users      []User     `json:"users"`
	Grants     []Grant    `json:"grants"`
	PHPMyAdmin PHPMyAdmin `json:"phpmyadmin"`
	Error      string     `json:"error,omitempty"`
}

func Collect() Admin {
	admin := Admin{Databases: []Database{}, Users: []User{}, Grants: []Grant{}, PHPMyAdmin: phpMyAdminStatus()}
	schemas, err := Query("SELECT SCHEMA_NAME FROM information_schema.SCHEMATA ORDER BY SCHEMA_NAME")
	if err != nil {
		admin.Error = err.Error()
		return admin
	}
	for _, line := range lines(schemas) {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		_, system := systemDBs[strings.ToLower(name)]
		admin.Databases = append(admin.Databases, Database{Name: name, System: system})
	}
	users, err := Query("SELECT User, Host FROM mysql.user ORDER BY User, Host")
	if err != nil {
		admin.Error = err.Error()
		return admin
	}
	for _, line := range lines(users) {
		name, host, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		admin.Users = append(admin.Users, User{Name: name, Host: host, System: systemAccount(name, host)})
	}
	grants, err := Query("SELECT GRANTEE, TABLE_SCHEMA, PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES ORDER BY GRANTEE, TABLE_SCHEMA, PRIVILEGE_TYPE")
	if err != nil {
		admin.Error = err.Error()
		return admin
	}
	grouped := map[string]*Grant{}
	var order []string
	for _, line := range lines(grants) {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			continue
		}
		user, host, ok := parseGrantee(parts[0])
		if !ok {
			continue
		}
		key := user + "\x00" + host + "\x00" + parts[1]
		item, found := grouped[key]
		if !found {
			item = &Grant{User: user, Host: host, Database: parts[1], System: systemAccount(user, host) || systemDatabase(parts[1])}
			grouped[key] = item
			order = append(order, key)
		}
		item.Privileges = append(item.Privileges, parts[2])
	}
	for _, key := range order {
		admin.Grants = append(admin.Grants, *grouped[key])
	}
	return admin
}

func CollectJSON() (string, error) {
	b, err := json.Marshal(Collect())
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Query(sql string) (string, error) {
	if strings.ContainsAny(sql, "\r\n\x00") {
		return "", fmt.Errorf("invalid mysql statement")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "mysql", "--protocol=socket", "-N", "-B", "--default-character-set=utf8mb4")
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			if errors.Is(err, exec.ErrNotFound) {
				return "", fmt.Errorf("mysql client was not found")
			}
			return "", fmt.Errorf("mysql command failed")
		}
		return "", mysqlError(text)
	}
	return text, nil
}

func CreateDatabase(name string) error {
	sql, err := CreateDatabaseSQL(name)
	if err != nil {
		return err
	}
	_, err = Query(sql)
	return err
}

func DropDatabase(name string) error {
	sql, err := DropDatabaseSQL(name)
	if err != nil {
		return err
	}
	_, err = Query(sql)
	return err
}

func CreateUser(name, host, password string) error {
	sql, err := CreateUserSQL(name, host, password)
	if err != nil {
		return err
	}
	if _, err := Query(sql); err != nil {
		return err
	}
	_, _ = Query("FLUSH PRIVILEGES")
	return nil
}

func DropUser(name, host string) error {
	sql, err := DropUserSQL(name, host)
	if err != nil {
		return err
	}
	if _, err := Query(sql); err != nil {
		return err
	}
	_, _ = Query("FLUSH PRIVILEGES")
	return nil
}

func SetPassword(name, host, password string) error {
	sql, err := PasswordSQL(name, host, password)
	if err != nil {
		return err
	}
	if _, err := Query(sql); err != nil {
		return err
	}
	_, _ = Query("FLUSH PRIVILEGES")
	return nil
}

func GrantPrivileges(name, host, database string, privs []string) error {
	revoke, err := RevokeSQL(name, host, database)
	if err != nil {
		return err
	}
	grant, err := GrantSQL(name, host, database, privs)
	if err != nil {
		return err
	}
	_, _ = Query(revoke)
	if _, err := Query(grant); err != nil {
		return err
	}
	_, _ = Query("FLUSH PRIVILEGES")
	return nil
}

func Revoke(name, host, database string) error {
	sql, err := RevokeSQL(name, host, database)
	if err != nil {
		return err
	}
	if _, err := Query(sql); err != nil {
		return err
	}
	_, _ = Query("FLUSH PRIVILEGES")
	return nil
}

func ApplySettings(maxConn, wait int) error {
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
	body := fmt.Sprintf("# managed-by: n-manager\n[mysqld]\nbind-address = 127.0.0.1\nmysqlx-bind-address = 127.0.0.1\nmax_connections = %d\nwait_timeout = %d\ncharacter-set-server = utf8mb4\ncollation-server = utf8mb4_unicode_ci\n", maxConn, wait)
	if err := os.WriteFile(filepath.Join(dir, "zz-n-manager.cnf"), []byte(body), 0o644); err != nil {
		return err
	}
	_, _ = Query(fmt.Sprintf("SET GLOBAL max_connections = %d", maxConn))
	_, _ = Query(fmt.Sprintf("SET GLOBAL wait_timeout = %d", wait))
	return nil
}

func CreateDatabaseSQL(name string) (string, error) {
	if err := validate.MySQLDatabase(name); err != nil {
		return "", err
	}
	return "CREATE DATABASE " + quoteIdent(name) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", nil
}

func DropDatabaseSQL(name string) (string, error) {
	if err := validate.MySQLDatabase(name); err != nil {
		return "", err
	}
	return "DROP DATABASE " + quoteIdent(name), nil
}

func CreateUserSQL(name, host, password string) (string, error) {
	if err := accountSQL(name, host, password); err != nil {
		return "", err
	}
	return "CREATE USER " + account(name, host) + " IDENTIFIED BY " + sqlString(password), nil
}

func DropUserSQL(name, host string) (string, error) {
	if err := validate.MySQLAccount(name, host); err != nil {
		return "", err
	}
	return "DROP USER " + account(name, host), nil
}

func PasswordSQL(name, host, password string) (string, error) {
	if err := accountSQL(name, host, password); err != nil {
		return "", err
	}
	return "ALTER USER " + account(name, host) + " IDENTIFIED BY " + sqlString(password), nil
}

func GrantSQL(name, host, database string, privs []string) (string, error) {
	if err := validate.MySQLAccount(name, host); err != nil {
		return "", err
	}
	if err := validate.MySQLDatabase(database); err != nil {
		return "", err
	}
	if err := validate.MySQLPrivileges(privs); err != nil {
		return "", err
	}
	clause := strings.Join(privs, ", ")
	if len(privs) == 1 && privs[0] == "ALL" {
		clause = "ALL PRIVILEGES"
	}
	return "GRANT " + clause + " ON " + quoteIdent(database) + ".* TO " + account(name, host), nil
}

func RevokeSQL(name, host, database string) (string, error) {
	if err := validate.MySQLAccount(name, host); err != nil {
		return "", err
	}
	if err := validate.MySQLDatabase(database); err != nil {
		return "", err
	}
	return "REVOKE ALL PRIVILEGES ON " + quoteIdent(database) + ".* FROM " + account(name, host), nil
}

func PHPMyAdminConfig(socket string) (string, error) {
	if !phpSockRe.MatchString(socket) {
		return "", fmt.Errorf("invalid php-fpm socket")
	}
	include := "/etc/nginx/fastcgi.conf"
	if _, err := os.Stat(include); err != nil {
		if _, alt := os.Stat("/etc/nginx/fastcgi_params"); alt == nil {
			include = "/etc/nginx/fastcgi_params"
		}
	}
	var b strings.Builder
	b.WriteString("# managed-by: n-manager\n")
	b.WriteString("server {\n")
	b.WriteString("    listen 127.0.0.1:8088;\n")
	b.WriteString("    server_name localhost;\n")
	b.WriteString("    root /usr/share/phpmyadmin;\n")
	b.WriteString("    index index.php index.html;\n")
	b.WriteString("    client_max_body_size 64m;\n")
	b.WriteString("    location / {\n")
	b.WriteString("        try_files $uri $uri/ =404;\n")
	b.WriteString("    }\n")
	b.WriteString("    location ~ \\.php$ {\n")
	b.WriteString("        fastcgi_split_path_info ^(.+\\.php)(/.+)$;\n")
	fmt.Fprintf(&b, "        fastcgi_pass unix:%s;\n", socket)
	b.WriteString("        fastcgi_index index.php;\n")
	fmt.Fprintf(&b, "        include %s;\n", include)
	b.WriteString("    }\n")
	b.WriteString("}\n")
	if err := validate.NginxContent(b.String()); err != nil {
		return "", err
	}
	return b.String(), nil
}

func InstallPHPMyAdmin(ctx context.Context, mgr string) error {
	switch mgr {
	case "apt":
		if err := preseed(ctx); err != nil {
			return err
		}
		if err := installPkg(ctx, "apt-get", "phpmyadmin"); err != nil {
			return err
		}
		return installPkg(ctx, "apt-get", "php-fpm")
	case "dnf":
		if err := installPkg(ctx, "dnf", "phpMyAdmin"); err != nil {
			return err
		}
		return installPkg(ctx, "dnf", "php-fpm")
	default:
		return fmt.Errorf("unsupported package manager")
	}
}

func SetPHPMyAdmin(enabled bool) error {
	nginxLayout, nginxErr := nginx.Detect("")
	apacheTool := "/usr/sbin/a2enconf"
	if !enabled {
		apacheTool = "/usr/sbin/a2disconf"
	}
	_, apacheErr := os.Stat(apacheTool)
	if nginxErr == nil {
		return setNginxPHPMyAdmin(nginxLayout, enabled)
	}
	if apacheErr == nil {
		return setApachePHPMyAdmin(enabled)
	}
	return fmt.Errorf("install Nginx or Apache before changing phpMyAdmin")
}

func setNginxPHPMyAdmin(layout nginx.Layout, enabled bool) error {
	if enabled {
		if _, err := os.Stat("/usr/share/phpmyadmin/index.php"); err != nil {
			return fmt.Errorf("phpMyAdmin is not installed")
		}
		unit, err := phpFPMUnit()
		if err != nil {
			return err
		}
		content, err := PHPMyAdminConfig("/run/php/" + unit + ".sock")
		if err != nil {
			return err
		}
		if err := nginx.Apply(layout, phpMyAdminSite, content, true, nginx.LiveTest); err != nil {
			return err
		}
		if err := run("systemctl", "enable", "--now", unit); err != nil {
			return err
		}
		return run("systemctl", "reload", "nginx")
	}
	site, err := nginx.Read(layout, phpMyAdminSite)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := nginx.Apply(layout, phpMyAdminSite, site.Config, false, nginx.LiveTest); err != nil {
		return err
	}
	return run("systemctl", "reload", "nginx")
}

func setApachePHPMyAdmin(enabled bool) error {
	link := "/etc/apache2/conf-enabled/phpmyadmin.conf"
	_, err := os.Stat(link)
	if enabled && err == nil {
		return run("systemctl", "reload", "apache2")
	}
	if !enabled && os.IsNotExist(err) {
		return nil
	}
	if enabled {
		if _, statErr := os.Stat("/etc/apache2/conf-available/phpmyadmin.conf"); statErr != nil {
			return fmt.Errorf("phpMyAdmin is not installed")
		}
	}
	tool := "a2enconf"
	if !enabled {
		tool = "a2disconf"
	}
	if err := run(tool, "phpmyadmin"); err != nil {
		return err
	}
	return run("systemctl", "reload", "apache2")
}

func phpMyAdminStatus() PHPMyAdmin {
	status := PHPMyAdmin{}
	mgr, err := pkgmgr.Detect()
	if err == nil {
		names := []string{"phpmyadmin"}
		if mgr.ID == "dnf" {
			names = []string{"phpMyAdmin", "phpmyadmin"}
		}
		for _, name := range names {
			installed, version, qerr := pkgmgr.Installed(mgr.ID, name)
			if qerr == nil && installed {
				status.Installed = true
				status.Version = version
				break
			}
		}
	}
	if _, err := os.Stat("/etc/nginx/sites-enabled/phpmyadmin.conf"); err == nil {
		status.Enabled = true
		status.Mode = "nginx"
	} else if _, err := os.Stat("/etc/nginx/conf.d/phpmyadmin.conf"); err == nil {
		status.Enabled = true
		status.Mode = "nginx"
	}
	if _, err := os.Stat("/etc/apache2/conf-enabled/phpmyadmin.conf"); err == nil {
		status.Enabled = true
		if status.Mode == "" {
			status.Mode = "apache"
		}
	}
	if status.Mode == "" {
		if _, err := nginx.Detect(""); err == nil {
			status.Mode = "nginx"
		} else if _, err := os.Stat("/usr/sbin/a2enconf"); err == nil {
			status.Mode = "apache"
		}
	}
	return status
}

func phpFPMUnit() (string, error) {
	var matches []string
	for _, pattern := range []string{"/usr/lib/systemd/system/php*-fpm.service", "/lib/systemd/system/php*-fpm.service"} {
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", fmt.Errorf("php-fpm is not installed")
	}
	unit := strings.TrimSuffix(filepath.Base(matches[len(matches)-1]), ".service")
	if !phpUnitRe.MatchString(unit) {
		return "", fmt.Errorf("php-fpm unit was not recognized")
	}
	return unit, nil
}

func preseed(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "debconf-set-selections")
	cmd.Stdin = strings.NewReader("phpmyadmin phpmyadmin/dbconfig-install boolean false\nphpmyadmin phpmyadmin/reconfigure-webserver multiselect\n")
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

func installPkg(ctx context.Context, tool, pkg string) error {
	if tool != "apt-get" && tool != "dnf" {
		return fmt.Errorf("unsupported package manager")
	}
	if pkg != "phpmyadmin" && pkg != "php-fpm" && pkg != "phpMyAdmin" {
		return fmt.Errorf("invalid package install request")
	}
	cmd := exec.CommandContext(ctx, tool, "install", "-y", pkg)
	if tool == "apt-get" {
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	}
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

func run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func accountSQL(name, host, password string) error {
	if err := validate.MySQLAccount(name, host); err != nil {
		return err
	}
	return validate.FTPPassword(password)
}

func account(name, host string) string {
	return sqlString(name) + "@" + sqlString(host)
}

func quoteIdent(name string) string {
	return "`" + name + "`"
}

func sqlString(value string) string {
	var b bytes.Buffer
	b.WriteByte('\'')
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteByte(value[i])
		default:
			b.WriteByte(value[i])
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func systemAccount(name, host string) bool {
	if _, reserved := map[string]struct{}{"root": {}, "mysql": {}, "mysql.sys": {}, "mysql.session": {}, "mysql.infoschema": {}, "debian-sys-maint": {}}[strings.ToLower(name)]; reserved {
		return true
	}
	return validate.MySQLAccount(name, host) != nil
}

func systemDatabase(name string) bool {
	_, ok := systemDBs[strings.ToLower(name)]
	return ok
}

func parseGrantee(value string) (string, string, bool) {
	if len(value) < 5 || value[0] != '\'' || !strings.HasSuffix(value, "'") {
		return "", "", false
	}
	rest := value[1 : len(value)-1]
	user, host, ok := strings.Cut(rest, "'@'")
	if !ok || user == "" {
		return "", "", false
	}
	return user, host, true
}

func lines(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func mysqlError(text string) error {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERROR ") {
			return fmt.Errorf("%s", line)
		}
	}
	return fmt.Errorf("mysql command failed")
}
