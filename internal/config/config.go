package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Host    string `yaml:"host"`
		Port    int    `yaml:"port"`
		TLSCert string `yaml:"tls_cert"`
		TLSKey  string `yaml:"tls_key"`
	} `yaml:"server"`
	Security struct {
		AdminPasswordHash string `yaml:"admin_password_hash"`
		SessionSecret     string `yaml:"session_secret"`
		SessionTTLMinutes int    `yaml:"session_ttl_minutes"`
		CookieSecure      string `yaml:"cookie_secure"`
		DryRun            bool   `yaml:"dry_run"`
	} `yaml:"security"`
	Database struct {
		Driver string `yaml:"driver"`
		Path   string `yaml:"path"`
	} `yaml:"database"`
	Monitoring struct {
		RefreshInterval int `yaml:"refresh_interval"`
	} `yaml:"monitoring"`
	Web struct {
		Server string `yaml:"server"`
	} `yaml:"web"`
	Privileged struct {
		Command string `yaml:"command"`
	} `yaml:"privileged"`
}

type File struct {
	mu   sync.Mutex
	Path string
	Data Config
}

func ResolvePath(flag string) string {
	if flag != "" {
		return flag
	}
	if v := os.Getenv("NMANAGER_CONFIG"); v != "" {
		return v
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	return "/etc/n-manager/config.yaml"
}

func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return &File{Path: path, Data: cfg}, nil
}

func (c *Config) normalize() error {
	if c.Server.Host == "" {
		c.Server.Host = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if _, err := bcrypt.Cost([]byte(c.Security.AdminPasswordHash)); err != nil {
		return fmt.Errorf("security.admin_password_hash must be a bcrypt hash")
	}
	if len(c.Security.SessionSecret) < 32 {
		return fmt.Errorf("security.session_secret must be at least 32 characters")
	}
	if c.Security.SessionTTLMinutes <= 0 {
		c.Security.SessionTTLMinutes = 720
	}
	if c.Security.SessionTTLMinutes > 24*60*30 {
		return fmt.Errorf("security.session_ttl_minutes is too large")
	}
	switch c.Security.CookieSecure {
	case "", "auto", "true", "false":
		if c.Security.CookieSecure == "" {
			c.Security.CookieSecure = "auto"
		}
	default:
		return fmt.Errorf("security.cookie_secure must be auto, true, or false")
	}
	if c.Database.Driver == "" {
		c.Database.Driver = "sqlite"
	}
	if c.Database.Driver != "sqlite" {
		return fmt.Errorf("unsupported database driver %q", c.Database.Driver)
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.Monitoring.RefreshInterval <= 0 {
		c.Monitoring.RefreshInterval = 5
	}
	if c.Monitoring.RefreshInterval < 1 || c.Monitoring.RefreshInterval > 300 {
		return fmt.Errorf("monitoring.refresh_interval must be between 1 and 300 seconds")
	}
	switch c.Web.Server {
	case "", "nginx":
		c.Web.Server = "nginx"
	case "apache":
	default:
		return fmt.Errorf("web.server must be nginx or apache")
	}
	if err := validateBind(c.Server.Host, c.Server.TLSCert, c.Server.TLSKey); err != nil {
		return err
	}
	return nil
}

func validateBind(host, cert, key string) error {
	if isLoopback(host) {
		return nil
	}
	if cert == "" || key == "" {
		return fmt.Errorf("remote bind %s requires server.tls_cert and server.tls_key", host)
	}
	if _, err := os.Stat(cert); err != nil {
		return fmt.Errorf("tls cert: %w", err)
	}
	if _, err := os.Stat(key); err != nil {
		return fmt.Errorf("tls key: %w", err)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) SessionTTL() time.Duration {
	return time.Duration(c.Security.SessionTTLMinutes) * time.Minute
}

func (c Config) Addr() string {
	return net.JoinHostPort(c.Server.Host, fmt.Sprintf("%d", c.Server.Port))
}

func (f *File) Snapshot() Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Data
}

func (f *File) SetRefreshInterval(seconds int) error {
	if seconds < 1 || seconds > 300 {
		return fmt.Errorf("refresh interval must be between 1 and 300 seconds")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data.Monitoring.RefreshInterval = seconds
	return f.saveLocked()
}

func (f *File) SetWebServer(server string) error {
	switch server {
	case "nginx", "apache":
	default:
		return fmt.Errorf("web server must be nginx or apache")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data.Web.Server = server
	return f.saveLocked()
}

func (f *File) SetSessionTTL(minutes int) error {
	if minutes < 5 || minutes > 24*60*30 {
		return fmt.Errorf("session ttl must be between 5 minutes and 30 days")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data.Security.SessionTTLMinutes = minutes
	return f.saveLocked()
}

func (f *File) SetBind(host string, port int, cert, key string) (restart bool, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return false, fmt.Errorf("host is required")
	}
	if port < 1 || port > 65535 {
		return false, fmt.Errorf("port must be between 1 and 65535")
	}
	if !isLoopback(host) && (cert == "" || key == "") {
		return false, fmt.Errorf("remote access requires tls_cert and tls_key")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	changed := host != f.Data.Server.Host || port != f.Data.Server.Port || cert != f.Data.Server.TLSCert || key != f.Data.Server.TLSKey
	f.Data.Server.Host = host
	f.Data.Server.Port = port
	f.Data.Server.TLSCert = cert
	f.Data.Server.TLSKey = key
	if err := f.saveLocked(); err != nil {
		return false, err
	}
	return changed, nil
}

func (f *File) SetPasswordHash(hash string) error {
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return fmt.Errorf("invalid bcrypt hash")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data.Security.AdminPasswordHash = hash
	return f.saveLocked()
}

func (f *File) saveLocked() error {
	b, err := yaml.Marshal(f.Data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(f.Path)
	tmp, err := os.CreateTemp(dir, "config-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil {
		os.Remove(tmpName)
		return werr
	}
	if cerr != nil {
		os.Remove(tmpName)
		return cerr
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, f.Path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
