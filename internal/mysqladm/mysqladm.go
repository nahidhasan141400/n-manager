package mysqladm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

var systemDBs = map[string]struct{}{"mysql": {}, "information_schema": {}, "performance_schema": {}, "sys": {}}

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

type Admin struct {
	Databases []Database `json:"databases"`
	Users     []User     `json:"users"`
	Grants    []Grant    `json:"grants"`
	Error     string     `json:"error,omitempty"`
}

func Collect(ctx context.Context, _ string) Admin {
	admin := Admin{Databases: []Database{}, Users: []User{}, Grants: []Grant{}}
	schemas, err := Query(ctx, "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA ORDER BY SCHEMA_NAME")
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
	users, err := Query(ctx, "SELECT User, Host FROM mysql.user ORDER BY User, Host")
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
	grants, err := Query(ctx, "SELECT GRANTEE, TABLE_SCHEMA, PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES ORDER BY GRANTEE, TABLE_SCHEMA, PRIVILEGE_TYPE")
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

func CollectJSON(ctx context.Context, kind string) (string, error) {
	b, err := json.Marshal(Collect(ctx, kind))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Query(ctx context.Context, sql string) (string, error) {
	if strings.ContainsAny(sql, "\r\n\x00") {
		return "", fmt.Errorf("invalid mysql statement")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := remote.Run(ctx, "mysql", []string{"--protocol=socket", "-N", "-B", "--default-character-set=utf8mb4"}, sql)
	text := strings.TrimSpace(out)
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("mysql command failed")
		}
		return "", mysqlError(text)
	}
	return text, nil
}

func CreateDatabase(ctx context.Context, name string) error {
	sql, err := CreateDatabaseSQL(name)
	if err != nil {
		return err
	}
	_, err = Query(ctx, sql)
	return err
}

func DropDatabase(ctx context.Context, name string) error {
	sql, err := DropDatabaseSQL(name)
	if err != nil {
		return err
	}
	_, err = Query(ctx, sql)
	return err
}

func ExportDatabase(ctx context.Context, name string) (string, error) {
	if err := validate.MySQLDatabase(name); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	base := []string{"--protocol=socket", "--default-character-set=utf8mb4", "--single-transaction", "--routines", "--triggers", "--skip-comments", name}
	args := append(append([]string{}, base[:len(base)-1]...), "--no-tablespaces", "--set-gtid-purged=OFF", name)
	out, err := remote.Run(ctx, "mysqldump", args, "")
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unknown option") {
		out, err = remote.Run(ctx, "mysqldump", base, "")
	}
	if err != nil {
		return "", fmt.Errorf("could not export %s: %s", name, shortMySQL(err.Error()))
	}
	if err := validate.MySQLDump(out); err != nil {
		return "", fmt.Errorf("could not export %s: %w", name, err)
	}
	return out, nil
}

func ImportDatabase(ctx context.Context, name, sql string) error {
	if err := validate.MySQLDatabase(name); err != nil {
		return err
	}
	if err := validate.MySQLDump(sql); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	_, err := remote.Run(ctx, "mysql", []string{"--protocol=socket", "--default-character-set=utf8mb4", name}, sql)
	if err != nil {
		return fmt.Errorf("could not import %s: %s", name, shortMySQL(err.Error()))
	}
	return nil
}

func shortMySQL(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 240 {
		msg = msg[:240]
	}
	if msg == "" {
		return "mysql command failed"
	}
	return msg
}

func CreateUser(ctx context.Context, name, host, password string) error {
	sql, err := CreateUserSQL(name, host, password)
	if err != nil {
		return err
	}
	if _, err := Query(ctx, sql); err != nil {
		return err
	}
	_, _ = Query(ctx, "FLUSH PRIVILEGES")
	return nil
}

func DropUser(ctx context.Context, name, host string) error {
	sql, err := DropUserSQL(name, host)
	if err != nil {
		return err
	}
	if _, err := Query(ctx, sql); err != nil {
		return err
	}
	_, _ = Query(ctx, "FLUSH PRIVILEGES")
	return nil
}

func SetPassword(ctx context.Context, name, host, password string) error {
	sql, err := PasswordSQL(name, host, password)
	if err != nil {
		return err
	}
	if _, err := Query(ctx, sql); err != nil {
		return err
	}
	_, _ = Query(ctx, "FLUSH PRIVILEGES")
	return nil
}

func GrantPrivileges(ctx context.Context, name, host, database string, privs []string) error {
	revoke, err := RevokeSQL(name, host, database)
	if err != nil {
		return err
	}
	grant, err := GrantSQL(name, host, database, privs)
	if err != nil {
		return err
	}
	_, _ = Query(ctx, revoke)
	if _, err := Query(ctx, grant); err != nil {
		return err
	}
	_, _ = Query(ctx, "FLUSH PRIVILEGES")
	return nil
}

func Revoke(ctx context.Context, name, host, database string) error {
	sql, err := RevokeSQL(name, host, database)
	if err != nil {
		return err
	}
	if _, err := Query(ctx, sql); err != nil {
		return err
	}
	_, _ = Query(ctx, "FLUSH PRIVILEGES")
	return nil
}

func ApplySettings(ctx context.Context, maxConn, wait int) error {
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
	body := fmt.Sprintf("# managed-by: n-manager\n[mysqld]\nbind-address = 127.0.0.1\nmysqlx-bind-address = 127.0.0.1\nmax_connections = %d\nwait_timeout = %d\ncharacter-set-server = utf8mb4\ncollation-server = utf8mb4_unicode_ci\n", maxConn, wait)
	if err := remote.WriteFile(ctx, filepath.Join(dir, "zz-n-manager.cnf"), []byte(body), 0o644); err != nil {
		return err
	}
	_, _ = Query(ctx, fmt.Sprintf("SET GLOBAL max_connections = %d", maxConn))
	_, _ = Query(ctx, fmt.Sprintf("SET GLOBAL wait_timeout = %d", wait))
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

func run(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_, err := remote.Run(ctx, name, args, "")
	return err
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
