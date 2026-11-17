package ftp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"n-manager/internal/pkgmgr"
	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

const (
	ConfPath    = "/etc/vsftpd.conf"
	UserList    = "/etc/vsftpd.userlist"
	UserConfDir = "/etc/vsftpd.user_conf"
	XferLog     = "/var/log/vsftpd.log"
)

type Status struct {
	Installed   bool      `json:"installed"`
	Version     string    `json:"version"`
	Active      string    `json:"active"`
	Enabled     string    `json:"enabled"`
	Exists      bool      `json:"unit_exists"`
	Port        int       `json:"port"`
	PasvMin     int       `json:"pasv_min"`
	PasvMax     int       `json:"pasv_max"`
	PasvAddr    string    `json:"pasv_address"`
	Write       bool      `json:"write"`
	Umask       string    `json:"umask"`
	Idle        int       `json:"idle_timeout"`
	DataTimeout int       `json:"data_timeout"`
	MaxClients  int       `json:"max_clients"`
	MaxPerIP    int       `json:"max_per_ip"`
	MaxRate     int       `json:"max_rate"`
	Banner      string    `json:"banner"`
	TransferLog bool      `json:"transfer_log"`
	TLS         bool      `json:"tls"`
	TLSCert     string    `json:"tls_cert"`
	TLSKey      string    `json:"tls_key"`
	Users       []User    `json:"users"`
	Sessions    []Session `json:"sessions"`
}

type User struct {
	Name   string `json:"name"`
	Home   string `json:"home"`
	Locked *bool  `json:"locked"`
	Write  bool   `json:"write"`
}

type Session struct {
	Local string `json:"local"`
	Peer  string `json:"peer"`
}

func Read(ctx context.Context) Status {
	spec := validate.FTPDefaults()
	if cfg, err := parseConf(ctx, ConfPath); err == nil {
		spec = cfg
	}
	st := statusFrom(spec)
	st.Users = []User{}
	st.Sessions = []Session{}
	st.Active, st.Enabled = "not-found", "unknown"
	mgr, err := pkgmgr.Detect(ctx)
	if err == nil {
		installed, version, qerr := pkgmgr.Installed(ctx, mgr.ID, "vsftpd")
		if qerr == nil && installed {
			st.Installed = true
			st.Version = version
		}
	}
	st.Active, st.Enabled, st.Exists = probeUnit(ctx, "vsftpd")
	st.Users = listUsers(ctx, UserList, "/etc/passwd", "/etc/shadow", spec.Write)
	st.Sessions = ParseSessions(commandOut(ctx, "ss", "-Htn", "state", "established"), spec.Port)
	return st
}

func statusFrom(spec validate.FTPApply) Status {
	return Status{
		Port: spec.Port, PasvMin: spec.PasvMin, PasvMax: spec.PasvMax, PasvAddr: spec.PasvAddress,
		Write: spec.Write, Umask: spec.Umask, Idle: spec.Idle, DataTimeout: spec.DataTimeout,
		MaxClients: spec.MaxClients, MaxPerIP: spec.MaxPerIP, MaxRate: spec.MaxRate,
		Banner: spec.Banner, TransferLog: spec.TransferLog, TLS: spec.TLS, TLSCert: spec.TLSCert, TLSKey: spec.TLSKey,
	}
}

func Render(spec validate.FTPApply) string {
	var b strings.Builder
	b.WriteString("# managed-by: n-manager\n")
	b.WriteString("listen=YES\nlisten_ipv6=NO\n")
	fmt.Fprintf(&b, "listen_port=%d\n", spec.Port)
	b.WriteString("anonymous_enable=NO\nlocal_enable=YES\n")
	fmt.Fprintf(&b, "write_enable=%s\n", yesNo(spec.Write))
	b.WriteString("chroot_local_user=YES\nallow_writeable_chroot=YES\n")
	fmt.Fprintf(&b, "local_umask=%s\n", spec.Umask)
	fmt.Fprintf(&b, "idle_session_timeout=%d\n", spec.Idle)
	fmt.Fprintf(&b, "data_connection_timeout=%d\n", spec.DataTimeout)
	fmt.Fprintf(&b, "max_clients=%d\n", spec.MaxClients)
	fmt.Fprintf(&b, "max_per_ip=%d\n", spec.MaxPerIP)
	fmt.Fprintf(&b, "local_max_rate=%d\n", spec.MaxRate)
	if spec.Banner != "" {
		fmt.Fprintf(&b, "ftpd_banner=%s\n", spec.Banner)
	}
	b.WriteString("pam_service_name=vsftpd\n")
	b.WriteString("userlist_enable=YES\nuserlist_file=/etc/vsftpd.userlist\nuserlist_deny=NO\n")
	fmt.Fprintf(&b, "user_config_dir=%s\n", UserConfDir)
	b.WriteString("pasv_enable=YES\n")
	fmt.Fprintf(&b, "pasv_min_port=%d\npasv_max_port=%d\n", spec.PasvMin, spec.PasvMax)
	if spec.PasvAddress != "" {
		fmt.Fprintf(&b, "pasv_address=%s\n", spec.PasvAddress)
	}
	fmt.Fprintf(&b, "xferlog_enable=%s\n", yesNo(spec.TransferLog))
	if spec.TransferLog {
		fmt.Fprintf(&b, "xferlog_std_format=YES\nxferlog_file=%s\n", XferLog)
	}
	fmt.Fprintf(&b, "ssl_enable=%s\n", yesNo(spec.TLS))
	if spec.TLS {
		b.WriteString("allow_anon_ssl=NO\nforce_local_logins_ssl=YES\nforce_local_data_ssl=YES\n")
		b.WriteString("ssl_sslv2=NO\nssl_sslv3=NO\nssl_tlsv1=YES\nrequire_ssl_reuse=NO\n")
		fmt.Fprintf(&b, "rsa_cert_file=%s\nrsa_private_key_file=%s\n", spec.TLSCert, spec.TLSKey)
	}
	return b.String()
}

func RenderUser(write bool) string {
	return "# managed-by: n-manager\nwrite_enable=" + yesNo(write) + "\n"
}

func UserConfPath(name string) (string, error) {
	if err := validate.FTPName(name); err != nil {
		return "", err
	}
	return filepath.Join(UserConfDir, name), nil
}

func Homes(ctx context.Context) []string {
	return ListHomes(ctx, []string{"/home", "/srv", "/var/www"}, 4, 300)
}

var skipHomeDirs = map[string]struct{}{
	"node_modules": {}, "vendor": {}, "dist": {}, "build": {},
	"__pycache__": {}, "target": {}, "cache": {}, "Cache": {},
}

func ListHomes(ctx context.Context, roots []string, maxDepth, maxCount int) []string {
	var paths []string
	for _, root := range roots {
		info, err := remote.Stat(ctx, root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		walkHomes(ctx, root, 1, maxDepth, maxCount, &paths)
	}
	sort.Strings(paths)
	if paths == nil {
		return []string{}
	}
	return paths
}

func walkHomes(ctx context.Context, dir string, depth, maxDepth, maxCount int, paths *[]string) {
	if depth > maxDepth || len(*paths) >= maxCount {
		return
	}
	entries, err := remote.ReadDir(ctx, dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || strings.HasPrefix(name, ".") || !entry.IsDir() {
			continue
		}
		if _, skip := skipHomeDirs[name]; skip {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if len(*paths) >= maxCount {
			return
		}
		path := filepath.Join(dir, name)
		info, err := remote.Stat(ctx, path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		*paths = append(*paths, path)
		walkHomes(ctx, path, depth+1, maxDepth, maxCount, paths)
	}
}

func yesNo(on bool) string {
	if on {
		return "YES"
	}
	return "NO"
}

func parseConf(ctx context.Context, path string) (validate.FTPApply, error) {
	b, err := remote.ReadFile(ctx, path)
	if err != nil {
		return validate.FTPApply{}, err
	}
	spec := validate.FTPDefaults()
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "listen_port":
			fmt.Sscanf(val, "%d", &spec.Port)
		case "pasv_min_port":
			fmt.Sscanf(val, "%d", &spec.PasvMin)
		case "pasv_max_port":
			fmt.Sscanf(val, "%d", &spec.PasvMax)
		case "pasv_address":
			spec.PasvAddress = val
		case "write_enable":
			spec.Write = strings.EqualFold(val, "YES")
		case "local_umask":
			spec.Umask = val
		case "idle_session_timeout":
			fmt.Sscanf(val, "%d", &spec.Idle)
		case "data_connection_timeout":
			fmt.Sscanf(val, "%d", &spec.DataTimeout)
		case "max_clients":
			fmt.Sscanf(val, "%d", &spec.MaxClients)
		case "max_per_ip":
			fmt.Sscanf(val, "%d", &spec.MaxPerIP)
		case "local_max_rate":
			fmt.Sscanf(val, "%d", &spec.MaxRate)
		case "ftpd_banner":
			spec.Banner = val
		case "xferlog_enable":
			spec.TransferLog = strings.EqualFold(val, "YES")
		case "ssl_enable":
			spec.TLS = strings.EqualFold(val, "YES")
		case "rsa_cert_file":
			spec.TLSCert = val
		case "rsa_private_key_file":
			spec.TLSKey = val
		}
	}
	return spec, nil
}

func listUsers(ctx context.Context, userlist, passwd, shadow string, globalWrite bool) []User {
	b, err := remote.ReadFile(ctx, userlist)
	if err != nil {
		return []User{}
	}
	homes := passwdHomes(ctx, passwd)
	locked := shadowLocked(ctx, shadow)
	var users []User
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		users = append(users, User{Name: name, Home: homes[name], Locked: locked[name], Write: userWrite(ctx, name, globalWrite)})
	}
	if users == nil {
		return []User{}
	}
	return users
}

func userWrite(ctx context.Context, name string, fallback bool) bool {
	path, err := UserConfPath(name)
	if err != nil {
		return fallback
	}
	b, err := remote.ReadFile(ctx, path)
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "write_enable" {
			return strings.EqualFold(strings.TrimSpace(val), "YES")
		}
	}
	return fallback
}

func shadowLocked(ctx context.Context, path string) map[string]*bool {
	b, err := remote.ReadFile(ctx, path)
	if err != nil {
		return nil
	}
	out := map[string]*bool{}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 2 || fields[0] == "" {
			continue
		}
		hash := fields[1]
		locked := strings.HasPrefix(hash, "!") || strings.HasPrefix(hash, "*")
		out[fields[0]] = &locked
	}
	return out
}

func Sessions(ctx context.Context, port int) []Session {
	return ParseSessions(commandOut(ctx, "ss", "-Htn", "state", "established"), port)
}

func ParseSessions(text string, port int) []Session {
	var sessions []Session
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		localPort, ok := portOf(fields[2])
		if !ok || localPort != port {
			continue
		}
		sessions = append(sessions, Session{Local: fields[2], Peer: fields[3]})
		if len(sessions) == 50 {
			break
		}
	}
	if sessions == nil {
		return []Session{}
	}
	return sessions
}

func portOf(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 || i == len(addr)-1 {
		return 0, false
	}
	n, err := strconv.Atoi(addr[i+1:])
	if err != nil {
		return 0, false
	}
	return n, true
}

func Tail(ctx context.Context, path string, lines int) (string, error) {
	if err := validate.ReadablePath(path); err != nil {
		return "", err
	}
	return remote.ReadTail(ctx, path, lines)
}

func passwdHomes(ctx context.Context, path string) map[string]string {
	out := map[string]string{}
	b, err := remote.ReadFile(ctx, path)
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) >= 6 {
			out[fields[0]] = fields[5]
		}
	}
	return out
}

func probeUnit(ctx context.Context, unit string) (active, enabled string, exists bool) {
	load := strings.TrimSpace(commandOut(ctx, "systemctl", "show", unit, "-p", "LoadState", "--value"))
	if load == "" || load == "not-found" {
		return "not-found", "unknown", false
	}
	active = strings.TrimSpace(commandOut(ctx, "systemctl", "show", unit, "-p", "ActiveState", "--value"))
	enabled = strings.TrimSpace(commandOut(ctx, "systemctl", "is-enabled", unit))
	if fields := strings.Fields(enabled); len(fields) > 0 {
		enabled = fields[0]
	}
	if enabled == "" {
		enabled = "unknown"
	}
	return active, enabled, true
}

func commandOut(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, _ := remote.Run(ctx, name, args, "")
	return out
}

func NologinShell(ctx context.Context) string {
	if _, err := remote.Stat(ctx, "/usr/sbin/nologin"); err == nil {
		return "/usr/sbin/nologin"
	}
	if _, err := remote.Run(ctx, "sh", []string{"-c", "command -v nologin"}, ""); err == nil {
		return "nologin"
	}
	return "/bin/false"
}
