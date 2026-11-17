package mysqladm

import (
	"strings"
	"testing"

	"n-manager/internal/validate"
)

func TestDatabaseSQL(t *testing.T) {
	sql, err := CreateDatabaseSQL("app_db")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "CREATE DATABASE `app_db`") || !strings.Contains(sql, "utf8mb4_unicode_ci") {
		t.Fatalf("sql = %s", sql)
	}
	if _, err := DropDatabaseSQL("mysql"); err == nil {
		t.Fatal("expected reserved database to be rejected")
	}
	if _, err := DropDatabaseSQL("information_schema"); err == nil {
		t.Fatal("expected information_schema to be rejected")
	}
}

func TestUserSQLEscapesPassword(t *testing.T) {
	sql, err := CreateUserSQL("app", "localhost", "p@ss'w\\ord")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "IDENTIFIED BY 'p@ss\\'w\\\\ord'") {
		t.Fatalf("sql = %s", sql)
	}
	if _, err := DropUserSQL("root", "localhost"); err == nil {
		t.Fatal("expected root to be rejected")
	}
	if _, err := PasswordSQL("app", "10.0.0.8", "long-password"); err == nil {
		t.Fatal("expected remote host to be rejected")
	}
}

func TestGrantSQL(t *testing.T) {
	sql, err := GrantSQL("app", "%", "app_db", []string{"SELECT", "INSERT"})
	if err != nil {
		t.Fatal(err)
	}
	if sql != "GRANT SELECT, INSERT ON `app_db`.* TO 'app'@'%'" {
		t.Fatalf("sql = %s", sql)
	}
	all, err := GrantSQL("app", "127.0.0.1", "app_db", []string{"ALL"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all, "GRANT ALL PRIVILEGES ON `app_db`.*") {
		t.Fatalf("sql = %s", all)
	}
	if err := validate.MySQLPrivileges([]string{"ALL", "SELECT"}); err == nil {
		t.Fatal("expected combined ALL to be rejected")
	}
	revoke, err := RevokeSQL("app", "localhost", "app_db")
	if err != nil {
		t.Fatal(err)
	}
	if revoke != "REVOKE ALL PRIVILEGES ON `app_db`.* FROM 'app'@'localhost'" {
		t.Fatalf("sql = %s", revoke)
	}
}

func TestPHPMyAdminConfigStaysLocal(t *testing.T) {
	config, err := PHPMyAdminConfig("/run/php/php8.3-fpm.sock")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, "listen 127.0.0.1:8088;") {
		t.Fatalf("config = %s", config)
	}
	if !strings.Contains(config, "root /usr/share/phpmyadmin;") {
		t.Fatalf("config = %s", config)
	}
	if err := validate.NginxContent(config); err != nil {
		t.Fatal(err)
	}
	if _, err := PHPMyAdminConfig("/tmp/php.sock"); err == nil {
		t.Fatal("expected socket outside /run/php to be rejected")
	}
}
