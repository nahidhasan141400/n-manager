package apps

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	"n-manager/internal/pkgmgr"
	"n-manager/internal/priv"
	"n-manager/internal/remote"
	"n-manager/internal/systemd"
)

type Engine struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Installed bool              `json:"installed"`
	Package   string            `json:"package"`
	Version   string            `json:"version"`
	Unit      string            `json:"unit"`
	Active    string            `json:"active"`
	Enabled   string            `json:"enabled"`
	Exists    bool              `json:"unit_exists"`
	Config    map[string]string `json:"config"`
	Manager   string            `json:"package_manager"`
}

type spec struct {
	id       string
	label    string
	packages []string
	units    []string
}

var engines = map[string]spec{
	"mysql":    {id: "mysql", label: "MySQL", packages: []string{"mysql-server", "mariadb-server"}, units: []string{"mysql", "mariadb", "mysqld"}},
	"postgres": {id: "postgres", label: "PostgreSQL", packages: []string{"postgresql"}, units: []string{"postgresql"}},
}

func Database(ctx context.Context, id string) (Engine, error) {
	sp, ok := engines[id]
	if !ok {
		return Engine{}, fmt.Errorf("unknown database")
	}
	mgr, err := pkgmgr.Detect(ctx)
	if err != nil {
		return Engine{}, err
	}
	eng := Engine{ID: sp.id, Label: sp.label, Manager: mgr.ID, Config: map[string]string{}}
	for _, pkg := range sp.packages {
		installed, version, qerr := pkgmgr.Installed(ctx, mgr.ID, pkg)
		if qerr != nil {
			return eng, qerr
		}
		if installed {
			eng.Installed = true
			eng.Package = pkg
			eng.Version = version
			if pkg == "mariadb-server" {
				eng.Label = "MariaDB"
			}
			break
		}
	}
	if eng.Package == "" {
		eng.Package = sp.packages[0]
	}
	st := systemd.Probe(ctx, sp.units)
	eng.Unit = st.Unit
	eng.Active = st.Active
	eng.Enabled = st.Enabled
	eng.Exists = st.Exists
	if id == "mysql" {
		eng.Config = readMySQLConfig(ctx)
	} else {
		eng.Config = readPostgresConfig(ctx)
	}
	return eng, nil
}

func InstallDatabase(ctx context.Context, ex priv.Executor, id string) (string, error) {
	sp, ok := engines[id]
	if !ok {
		return "", fmt.Errorf("unknown database")
	}
	mgr, err := pkgmgr.Detect(ctx)
	if err != nil {
		return "", err
	}
	pkg := sp.packages[0]
	if len(sp.packages) > 1 && !pkgmgr.Available(ctx, mgr.ID, pkg) && pkgmgr.Available(ctx, mgr.ID, sp.packages[1]) {
		pkg = sp.packages[1]
	}
	unit := sp.units[0]
	if pkg == "mariadb-server" {
		unit = "mariadb"
	}
	var logs []string
	for _, step := range []struct {
		op   string
		args []string
	}{
		{"pkg-install", []string{mgr.ID, pkg}},
		{"db-bind-local", []string{id}},
		{"systemctl", []string{"enable", unit}},
		{"systemctl", []string{"restart", unit}},
	} {
		out, err := ex.Run(ctx, step.op, step.args, "")
		logs = append(logs, strings.TrimSpace(out))
		if err != nil {
			return strings.Join(logs, "\n"), err
		}
	}
	return strings.Join(logs, "\n"), nil
}

func Service(ctx context.Context, ex priv.Executor, id, action string, confirm bool) (string, error) {
	if action == "stop" || action == "disable" {
		if !confirm {
			return "", fmt.Errorf("confirmation is required")
		}
	}
	eng, err := Database(ctx, id)
	if err != nil {
		return "", err
	}
	unit := eng.Unit
	if unit == "" {
		unit = engines[id].units[0]
	}
	return systemd.Action(ctx, ex, action, unit)
}

func Logs(ctx context.Context, ex priv.Executor, id string) (string, error) {
	eng, err := Database(ctx, id)
	if err != nil {
		return "", err
	}
	return systemd.Logs(ctx, ex, eng.Unit, 200)
}

var (
	bindRe    = regexp.MustCompile(`(?m)^\s*bind-address\s*=\s*(\S+)`)
	portRe    = regexp.MustCompile(`(?m)^\s*port\s*=\s*(\d+)`)
	dataRe    = regexp.MustCompile(`(?m)^\s*datadir\s*=\s*(\S+)`)
	maxConnRe = regexp.MustCompile(`(?m)^\s*max_connections\s*=\s*(\d+)`)
	waitRe    = regexp.MustCompile(`(?m)^\s*wait_timeout\s*=\s*(\d+)`)
	listenRe  = regexp.MustCompile(`(?m)^\s*listen_addresses\s*=\s*(\S+)`)
	pgPortRe  = regexp.MustCompile(`(?m)^\s*port\s*=\s*(\d+)`)
	pgDataRe  = regexp.MustCompile(`(?m)^\s*data_directory\s*=\s*'?([^'\s#]+)'?`)
)

func readMySQLConfig(ctx context.Context) map[string]string {
	out := map[string]string{}
	paths := []string{"/etc/mysql/my.cnf", "/etc/my.cnf"}
	paths = append(paths, glob(ctx, "/etc/mysql/mysql.conf.d/*.cnf")...)
	paths = append(paths, glob(ctx, "/etc/mysql/mariadb.conf.d/*.cnf")...)
	paths = append(paths, glob(ctx, "/etc/my.cnf.d/*.cnf")...)
	for _, path := range paths {
		b, err := remote.ReadFile(ctx, path)
		if err != nil {
			continue
		}
		text := string(b)
		if m := bindRe.FindStringSubmatch(text); m != nil {
			out["bind_address"] = m[1]
		}
		if m := portRe.FindStringSubmatch(text); m != nil {
			out["port"] = m[1]
		}
		if m := dataRe.FindStringSubmatch(text); m != nil {
			out["datadir"] = m[1]
		}
		if m := maxConnRe.FindStringSubmatch(text); m != nil {
			out["max_connections"] = m[1]
		}
		if m := waitRe.FindStringSubmatch(text); m != nil {
			out["wait_timeout"] = m[1]
		}
	}
	if out["bind_address"] == "" {
		out["bind_address"] = "unknown"
	}
	return out
}

func readPostgresConfig(ctx context.Context) map[string]string {
	out := map[string]string{"listen_addresses": "unknown"}
	for _, path := range glob(ctx, "/etc/postgresql/*/*/postgresql.conf") {
		b, err := remote.ReadFile(ctx, path)
		if err != nil {
			continue
		}
		text := string(b)
		if m := listenRe.FindStringSubmatch(text); m != nil {
			out["listen_addresses"] = strings.Trim(m[1], `"'`)
		}
		if m := pgPortRe.FindStringSubmatch(text); m != nil {
			out["port"] = m[1]
		}
		if m := pgDataRe.FindStringSubmatch(text); m != nil {
			out["data_directory"] = m[1]
		}
		out["config"] = path
	}
	return out
}

func glob(ctx context.Context, pattern string) []string {
	m, err := remote.Glob(ctx, pattern)
	if err != nil {
		return nil
	}
	return m
}

func ReadConfigValue(path string, key string) string {
	b, err := remote.ReadFile(context.Background(), path)
	if err != nil {
		return ""
	}
	return configValue(string(b), key)
}

func configValue(text, key string) string {
	sc := bufio.NewScanner(strings.NewReader(text))
	prefix := key + "="
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
