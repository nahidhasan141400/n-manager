package validate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	siteNameRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)
	domainRe   = regexp.MustCompile(`^(?:\*\.)?(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	versionRe  = regexp.MustCompile(`^v?\d+(?:\.\d+){0,2}$`)
	npmNameRe  = regexp.MustCompile(`^(?:@[a-z0-9-~][a-z0-9-._~]*/)?[a-z0-9-~][a-z0-9-._~]*$`)
	proxyRe    = regexp.MustCompile(`^https?://[A-Za-z0-9._:-]{1,253}(?::\d{1,5})?(?:/[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*)?$`)
	includeRe  = regexp.MustCompile(`(?i)^\s*include\s+(\S+)`)
)

var allowedPackages = map[string]struct{}{
	"mysql-server":   {},
	"mariadb-server": {},
	"postgresql":     {},
	"nginx":          {},
	"apache2":        {},
	"httpd":          {},
	"certbot":        {},
	"vsftpd":         {},
	"php-fpm":        {},
	"ufw":            {},
	"git":            {},
}

var allowedUnits = map[string]struct{}{
	"mysql":      {},
	"mariadb":    {},
	"mysqld":     {},
	"postgresql": {},
	"nginx":      {},
	"apache2":    {},
	"httpd":      {},
	"vsftpd":     {},
}

var allowedActions = map[string]struct{}{
	"start":   {},
	"stop":    {},
	"restart": {},
	"reload":  {},
	"enable":  {},
	"disable": {},
}

func Operation(op string, args []string) error {
	switch op {
	case "pkg-install", "pkg-remove":
		if len(args) != 2 || (args[0] != "apt" && args[0] != "dnf") || !AllowedPackage(args[1]) {
			return fmt.Errorf("invalid package install request")
		}
	case "systemctl":
		if len(args) != 2 || !AllowedAction(args[0]) || !AllowedUnit(args[1]) {
			return fmt.Errorf("invalid service request")
		}
	case "journal":
		if len(args) != 2 || !AllowedUnit(args[0]) {
			return fmt.Errorf("invalid journal request")
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 || n > 500 {
			return fmt.Errorf("journal line count must be between 1 and 500")
		}
	case "read-file":
		if len(args) != 1 {
			return fmt.Errorf("invalid read request")
		}
		if err := ReadablePath(args[0]); err != nil {
			return err
		}
	case "nginx-apply":
		if len(args) != 0 {
			return fmt.Errorf("invalid nginx apply request")
		}
	case "nginx-enable", "nginx-disable", "nginx-delete", "nginx-cert-delete":
		if len(args) != 1 || !siteNameRe.MatchString(args[0]) {
			return fmt.Errorf("invalid site name")
		}
	case "nginx-cert-put":
		if len(args) != 1 || !siteNameRe.MatchString(args[0]) {
			return fmt.Errorf("invalid certificate name")
		}
	case "certbot-issue":
		if len(args) < 2 || len(args) > 11 {
			return fmt.Errorf("invalid certificate request")
		}
		if err := Email(args[0]); err != nil {
			return err
		}
		if err := CertbotDomains(args[1:]); err != nil {
			return err
		}
	case "certbot-renew":
		if len(args) != 0 {
			return fmt.Errorf("invalid certificate request")
		}
	case "db-bind-local":
		if len(args) != 1 || (args[0] != "mysql" && args[0] != "postgres") {
			return fmt.Errorf("invalid database bind request")
		}
	case "nvm-install":
		if len(args) != 0 {
			return fmt.Errorf("invalid nvm install request")
		}
	case "node-install", "node-uninstall", "node-default":
		if len(args) != 1 {
			return fmt.Errorf("invalid node version request")
		}
		if err := NodeVersion(args[0]); err != nil {
			return err
		}
	case "npm-global-install", "npm-global-uninstall":
		if len(args) != 1 {
			return fmt.Errorf("invalid npm package request")
		}
		if err := NPMPackage(args[0]); err != nil {
			return err
		}
	case "pm2":
		if err := PM2Operation(args); err != nil {
			return err
		}
	case "ftp-apply":
		if len(args) != 0 {
			return fmt.Errorf("invalid ftp request")
		}
	case "ftp-user-add":
		if len(args) != 2 {
			return fmt.Errorf("invalid ftp user request")
		}
		if err := FTPUser(args[0], args[1]); err != nil {
			return err
		}
	case "ftp-user-delete", "ftp-user-password":
		if len(args) != 1 {
			return fmt.Errorf("invalid ftp user request")
		}
		if err := FTPName(args[0]); err != nil {
			return err
		}
	case "ftp-user-lock":
		if len(args) != 2 || (args[1] != "lock" && args[1] != "unlock") {
			return fmt.Errorf("invalid ftp user request")
		}
		if err := FTPName(args[0]); err != nil {
			return err
		}
	case "ftp-user-write":
		if len(args) != 2 || (args[1] != "yes" && args[1] != "no") {
			return fmt.Errorf("invalid ftp user request")
		}
		if err := FTPName(args[0]); err != nil {
			return err
		}
	case "mysql-list":
		if len(args) > 1 || (len(args) == 1 && args[0] != "nginx" && args[0] != "apache") {
			return fmt.Errorf("invalid mysql request")
		}
	case "mysql-settings":
		if len(args) != 2 {
			return fmt.Errorf("invalid mysql settings")
		}
		if _, _, err := MySQLSettings(args[0], args[1]); err != nil {
			return err
		}
	case "mysql-db-create", "mysql-db-drop", "mysql-db-export", "mysql-db-import":
		if len(args) != 1 {
			return fmt.Errorf("invalid mysql database request")
		}
		if err := MySQLDatabase(args[0]); err != nil {
			return err
		}
	case "mysql-user-create", "mysql-user-drop", "mysql-user-password":
		if len(args) != 2 {
			return fmt.Errorf("invalid mysql user request")
		}
		if err := MySQLAccount(args[0], args[1]); err != nil {
			return err
		}
	case "mysql-grant":
		if len(args) < 4 {
			return fmt.Errorf("invalid mysql privilege request")
		}
		if err := MySQLAccount(args[0], args[1]); err != nil {
			return err
		}
		if err := MySQLDatabase(args[2]); err != nil {
			return err
		}
		if err := MySQLPrivileges(args[3:]); err != nil {
			return err
		}
	case "mysql-revoke":
		if len(args) != 3 {
			return fmt.Errorf("invalid mysql privilege request")
		}
		if err := MySQLAccount(args[0], args[1]); err != nil {
			return err
		}
		if err := MySQLDatabase(args[2]); err != nil {
			return err
		}
	case "firewall-status", "firewall-disable":
		if len(args) != 0 {
			return fmt.Errorf("invalid firewall request")
		}
	case "firewall-enable":
		if len(args) != 1 {
			return fmt.Errorf("invalid firewall request")
		}
		if err := FirewallPort(args[0]); err != nil {
			return err
		}
	case "firewall-allow":
		if len(args) != 7 {
			return fmt.Errorf("invalid firewall request")
		}
		port, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid firewall port")
		}
		sshPort, err := strconv.Atoi(args[6])
		if err != nil {
			return fmt.Errorf("invalid firewall port")
		}
		if err := FirewallAccess(args[0], args[1], port, args[3], args[4], args[5], sshPort); err != nil {
			return err
		}
	case "firewall-config":
		if len(args) != 5 {
			return fmt.Errorf("invalid firewall request")
		}
		if err := FirewallPolicy("incoming", args[0]); err != nil {
			return err
		}
		if err := FirewallPolicy("outgoing", args[1]); err != nil {
			return err
		}
		if err := FirewallLogging(args[2]); err != nil {
			return err
		}
		if args[3] != "yes" && args[3] != "no" {
			return fmt.Errorf("invalid firewall request")
		}
		if err := FirewallPort(args[4]); err != nil {
			return err
		}
	case "firewall-delete":
		if len(args) != 2 {
			return fmt.Errorf("invalid firewall request")
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 || n > 500 {
			return fmt.Errorf("invalid firewall rule")
		}
		if err := FirewallPort(args[1]); err != nil {
			return err
		}
	case "firewall-update":
		if len(args) != 8 {
			return fmt.Errorf("invalid firewall request")
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 || n > 500 {
			return fmt.Errorf("invalid firewall rule")
		}
		port, err := strconv.Atoi(args[3])
		if err != nil {
			return fmt.Errorf("invalid firewall port")
		}
		sshPort, err := strconv.Atoi(args[7])
		if err != nil {
			return fmt.Errorf("invalid firewall port")
		}
		if err := FirewallAccess(args[1], args[2], port, args[4], args[5], args[6], sshPort); err != nil {
			return err
		}
	case "git-config":
		if len(args) != 2 {
			return fmt.Errorf("invalid git identity")
		}
		if err := GitName(args[0]); err != nil {
			return err
		}
		if err := Email(args[1]); err != nil {
			return err
		}
	case "git-clone":
		if len(args) != 2 {
			return fmt.Errorf("invalid git clone request")
		}
		if err := GitURL(args[0]); err != nil {
			return err
		}
		if err := GitDest(args[1]); err != nil {
			return err
		}
	case "git-pull":
		if len(args) != 1 {
			return fmt.Errorf("invalid git repository")
		}
		if err := GitRepoPath(args[0]); err != nil {
			return err
		}
	case "git-github", "git-github-test":
		if len(args) != 0 {
			return fmt.Errorf("invalid github request")
		}
	default:
		return fmt.Errorf("unknown operation")
	}
	return nil
}

func AllowedPackage(name string) bool {
	_, ok := allowedPackages[name]
	return ok
}

var phpFPMUnitRe = regexp.MustCompile(`^php[0-9]+\.[0-9]+-fpm$`)

func AllowedUnit(name string) bool {
	if _, ok := allowedUnits[name]; ok {
		return true
	}
	return phpFPMUnitRe.MatchString(name)
}

func AllowedAction(name string) bool {
	_, ok := allowedActions[name]
	return ok
}

func NodeVersion(v string) error {
	switch v {
	case "node", "lts", "lts/*", "system":
		return nil
	}
	v = strings.TrimPrefix(v, "v")
	if versionRe.MatchString(v) && len(v) <= 32 {
		return nil
	}
	return fmt.Errorf("invalid node version")
}

var pm2NameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func PM2Operation(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("invalid pm2 request")
	}
	switch args[0] {
	case "install":
		if len(args) != 1 {
			return fmt.Errorf("invalid pm2 request")
		}
	case "start":
		if len(args) != 2 && len(args) != 3 {
			return fmt.Errorf("invalid pm2 request")
		}
		if err := PM2Name(args[1]); err != nil {
			return err
		}
		if len(args) == 3 {
			if err := PM2Script(args[2]); err != nil {
				return err
			}
		}
	case "stop", "restart", "delete", "logs":
		if len(args) != 2 {
			return fmt.Errorf("invalid pm2 request")
		}
		if err := PM2Name(args[1]); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid pm2 action")
	}
	return nil
}

func PM2Name(name string) error {
	if !pm2NameRe.MatchString(name) {
		return fmt.Errorf("invalid process name")
	}
	return nil
}

func PM2Script(path string) error {
	return checkPM2Script(path, pm2Roots())
}

func checkPM2Script(path string, roots []string) error {
	if path == "" || len(path) > 4096 || strings.Contains(path, "\x00") || !filepath.IsAbs(path) {
		return fmt.Errorf("script must be an absolute file path")
	}
	clean := filepath.Clean(path)
	if clean != path {
		return fmt.Errorf("script must be an absolute file path")
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if ext != ".js" && ext != ".mjs" && ext != ".cjs" {
		return fmt.Errorf("script must be a JavaScript file")
	}
	allowed := false
	for _, root := range roots {
		root = filepath.Clean(root)
		if clean == root || strings.HasPrefix(clean, root+string(os.PathSeparator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("script must be inside the home directory or a web root")
	}
	info, err := os.Stat(clean)
	if err != nil || info.IsDir() {
		return fmt.Errorf("script was not found")
	}
	return nil
}

func pm2Roots() []string {
	roots := []string{"/var/www", "/srv", "/var/lib/n-manager"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, home)
	}
	return roots
}

func NPMPackage(name string) error {
	if len(name) == 0 || len(name) > 214 || strings.HasPrefix(name, "-") || !npmNameRe.MatchString(name) {
		return fmt.Errorf("invalid npm package name")
	}
	return nil
}

func SiteName(name string) error {
	if !siteNameRe.MatchString(name) {
		return fmt.Errorf("site name must be a short lowercase identifier")
	}
	return nil
}

func Domain(name string) error {
	if name == "" || len(name) > 253 {
		return fmt.Errorf("invalid domain")
	}
	if net.ParseIP(name) != nil {
		return nil
	}
	if !domainRe.MatchString(name) {
		return fmt.Errorf("invalid domain %q", name)
	}
	return nil
}

func ProxyPass(raw string) error {
	if len(raw) > 300 || !proxyRe.MatchString(raw) {
		return fmt.Errorf("proxy address must be an http or https URL")
	}
	return nil
}

func PathUnder(path string, roots []string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != path {
		return fmt.Errorf("path must not contain relative segments")
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path is outside the allowed directories")
}

func WebRoot(path string) error {
	return PathUnder(path, []string{"/var/www", "/srv/www", "/srv"})
}

func CertPath(path string) error {
	return PathUnder(path, []string{"/etc/letsencrypt", "/etc/ssl", "/etc/nginx"})
}

func ReadablePath(path string) error {
	return PathUnder(path, []string{"/var/log/nginx", "/var/log/mysql", "/var/log/postgresql", "/var/log/mariadb", "/var/log/vsftpd.log", "/etc/nginx", "/etc/mysql", "/etc/vsftpd.conf", "/etc/vsftpd.userlist"})
}

var ftpNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

var reservedFTPUsers = map[string]struct{}{
	"root": {}, "daemon": {}, "bin": {}, "sys": {}, "sync": {}, "man": {}, "mail": {},
	"www-data": {}, "backup": {}, "nobody": {}, "sshd": {}, "nmanager": {}, "nmanager-deploy": {},
	"mysql": {}, "postgres": {}, "nginx": {},
}

var (
	ftpUmaskRe  = regexp.MustCompile(`^[0-7]{3}$`)
	ftpBannerRe = regexp.MustCompile(`^[A-Za-z0-9 .,:'()/_-]{0,120}$`)
)

type FTPApply struct {
	Port        int    `json:"port"`
	PasvMin     int    `json:"pasv_min"`
	PasvMax     int    `json:"pasv_max"`
	PasvAddress string `json:"pasv_address"`
	Write       bool   `json:"write"`
	Umask       string `json:"umask"`
	Idle        int    `json:"idle_timeout"`
	DataTimeout int    `json:"data_timeout"`
	MaxClients  int    `json:"max_clients"`
	MaxPerIP    int    `json:"max_per_ip"`
	MaxRate     int    `json:"max_rate"`
	Banner      string `json:"banner"`
	TransferLog bool   `json:"transfer_log"`
	TLS         bool   `json:"tls"`
	TLSCert     string `json:"tls_cert"`
	TLSKey      string `json:"tls_key"`
}

func FTPDefaults() FTPApply {
	return FTPApply{
		Port: 21, PasvMin: 40000, PasvMax: 40100,
		Write: true, Umask: "022", Idle: 600, DataTimeout: 120,
		MaxClients: 50, MaxPerIP: 5, TransferLog: true,
	}
}

func FTPConfig(spec FTPApply) error {
	if spec.Port != 21 && (spec.Port < 1024 || spec.Port > 65535) {
		return fmt.Errorf("ftp port must be 21 or between 1024 and 65535")
	}
	if spec.PasvMin < 1024 || spec.PasvMax > 65535 || spec.PasvMin >= spec.PasvMax || spec.PasvMax-spec.PasvMin > 100 {
		return fmt.Errorf("passive ports must be a range of at most 100 ports")
	}
	if spec.PasvAddress != "" && net.ParseIP(spec.PasvAddress) == nil {
		return fmt.Errorf("passive address must be an IP address")
	}
	if !ftpUmaskRe.MatchString(spec.Umask) {
		return fmt.Errorf("umask must be three octal digits")
	}
	if spec.Idle < 30 || spec.Idle > 86400 || spec.DataTimeout < 10 || spec.DataTimeout > 3600 {
		return fmt.Errorf("session timeouts are out of range")
	}
	if spec.MaxClients < 1 || spec.MaxClients > 5000 || spec.MaxPerIP < 1 || spec.MaxPerIP > 100 {
		return fmt.Errorf("client limits are out of range")
	}
	if spec.MaxRate < 0 || (spec.MaxRate > 0 && spec.MaxRate < 1024) || spec.MaxRate > 100*1024*1024 {
		return fmt.Errorf("transfer rate must be unlimited or between 1 KB/s and 100 MB/s")
	}
	if !ftpBannerRe.MatchString(spec.Banner) {
		return fmt.Errorf("banner contains unsupported characters")
	}
	if spec.TLS {
		if err := CertPath(spec.TLSCert); err != nil {
			return fmt.Errorf("TLS certificate must be inside /etc/ssl, /etc/letsencrypt, or /etc/nginx")
		}
		if err := CertPath(spec.TLSKey); err != nil {
			return fmt.Errorf("TLS key must be inside /etc/ssl, /etc/letsencrypt, or /etc/nginx")
		}
		if spec.TLSCert == spec.TLSKey {
			return fmt.Errorf("TLS certificate and key must be different files")
		}
	}
	return nil
}

func FTPApplyPayload(stdin string) (FTPApply, error) {
	var spec FTPApply
	if err := json.Unmarshal([]byte(stdin), &spec); err != nil {
		return spec, fmt.Errorf("invalid ftp payload")
	}
	if err := FTPConfig(spec); err != nil {
		return spec, err
	}
	return spec, nil
}

func FTPName(name string) error {
	if _, reserved := reservedFTPUsers[name]; reserved || !ftpNameRe.MatchString(name) {
		return fmt.Errorf("invalid ftp username")
	}
	return nil
}

func FTPHome(path string) error {
	roots := []string{"/home", "/srv", "/var/www"}
	if err := PathUnder(path, roots); err != nil {
		return fmt.Errorf("ftp home must be inside /home, /srv, or /var/www")
	}
	for _, root := range roots {
		if filepath.Clean(path) == root {
			return fmt.Errorf("choose a directory inside the allowed root")
		}
	}
	return nil
}

func FTPUser(name, home string) error {
	if err := FTPName(name); err != nil {
		return err
	}
	return FTPHome(home)
}

func FTPPassword(password string) error {
	if len(password) < 8 || len(password) > 128 || strings.ContainsAny(password, "\r\n\x00") {
		return fmt.Errorf("password must be 8 to 128 characters")
	}
	return nil
}

type SiteApply struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

type NginxCert struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

func NginxCertPayload(stdin string) error {
	var body NginxCert
	if err := json.Unmarshal([]byte(stdin), &body); err != nil {
		return fmt.Errorf("invalid certificate payload")
	}
	return NginxCertificate(body.Certificate, body.Key)
}

func NginxCertificate(certPEM, keyPEM string) error {
	if len(certPEM) == 0 || len(certPEM) > 32*1024 || len(keyPEM) == 0 || len(keyPEM) > 32*1024 {
		return fmt.Errorf("certificate and key must be PEM text under 32 KB")
	}
	if strings.ContainsRune(certPEM, 0) || strings.ContainsRune(keyPEM, 0) {
		return fmt.Errorf("certificate contains invalid characters")
	}
	block, rest := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" || len(rest) > 0 && strings.TrimSpace(string(rest)) != "" && !onlyPEM(rest) {
		return fmt.Errorf("certificate must be a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("certificate could not be read")
	}
	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil || !strings.Contains(keyBlock.Type, "PRIVATE KEY") {
		return fmt.Errorf("key must be a PEM private key")
	}
	private, err := parsePrivateKey(keyBlock)
	if err != nil {
		return fmt.Errorf("key could not be read")
	}
	if !samePublicKey(cert.PublicKey, publicKey(private)) {
		return fmt.Errorf("private key does not match the certificate")
	}
	return nil
}

func onlyPEM(rest []byte) bool {
	text := strings.TrimSpace(string(rest))
	return text == "" || strings.HasPrefix(text, "-----BEGIN")
}

func parsePrivateKey(block *pem.Block) (crypto.PrivateKey, error) {
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	default:
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		private, ok := key.(crypto.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("unsupported key")
		}
		return private, nil
	}
}

func publicKey(key crypto.PrivateKey) crypto.PublicKey {
	switch value := key.(type) {
	case *rsa.PrivateKey:
		return &value.PublicKey
	case *ecdsa.PrivateKey:
		return &value.PublicKey
	case ed25519.PrivateKey:
		return value.Public()
	default:
		return nil
	}
}

func samePublicKey(left, right crypto.PublicKey) bool {
	a, errA := x509.MarshalPKIXPublicKey(left)
	b, errB := x509.MarshalPKIXPublicKey(right)
	return errA == nil && errB == nil && string(a) == string(b)
}

func NginxApply(stdin string) (SiteApply, error) {
	var spec SiteApply
	if err := json.Unmarshal([]byte(stdin), &spec); err != nil {
		return spec, fmt.Errorf("invalid nginx payload")
	}
	if err := SiteName(spec.Name); err != nil {
		return spec, err
	}
	if err := NginxContent(spec.Content); err != nil {
		return spec, err
	}
	return spec, nil
}

var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

func Email(value string) error {
	if len(value) > 254 || !emailRe.MatchString(value) {
		return fmt.Errorf("invalid email address")
	}
	return nil
}

func CertbotDomains(domains []string) error {
	if len(domains) < 1 || len(domains) > 10 {
		return fmt.Errorf("choose between 1 and 10 domains")
	}
	seen := map[string]struct{}{}
	for _, domain := range domains {
		if strings.HasPrefix(domain, "*.") {
			return fmt.Errorf("wildcard domains need a DNS challenge")
		}
		if err := Domain(domain); err != nil {
			return err
		}
		if _, ok := seen[domain]; ok {
			return fmt.Errorf("duplicate domain")
		}
		seen[domain] = struct{}{}
	}
	return nil
}

func NginxBodySize(size string) error {
	if size == "" {
		return nil
	}
	if !regexp.MustCompile(`^[1-9][0-9]{0,4}[kKmMgG]?$`).MatchString(size) {
		return fmt.Errorf("body size must look like 1m or 20m")
	}
	return nil
}

func NginxContent(content string) error {
	if len(content) == 0 || len(content) > 64*1024 {
		return fmt.Errorf("nginx config must be between 1 and 65536 bytes")
	}
	if strings.ContainsRune(content, 0) {
		return fmt.Errorf("nginx config contains invalid characters")
	}
	if !strings.Contains(content, "# managed-by: n-manager") {
		return fmt.Errorf("only configurations created by N Manager can be changed")
	}
	if regexp.MustCompile(`(?i)\bload_module\b`).MatchString(content) {
		return fmt.Errorf("load_module is not allowed")
	}
	for _, line := range strings.Split(content, "\n") {
		m := includeRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		inc := strings.Trim(m[1], `"'`)
		if inc == "mime.types" || strings.HasPrefix(inc, "/etc/nginx/") {
			continue
		}
		return fmt.Errorf("include paths must stay inside /etc/nginx")
	}
	return nil
}

var (
	mysqlIdentRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	mysqlUserRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
	mysqlPrivs   = map[string]struct{}{
		"ALL": {}, "SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {},
		"CREATE": {}, "DROP": {}, "INDEX": {}, "ALTER": {}, "CREATE VIEW": {}, "SHOW VIEW": {},
	}
	reservedMySQLDatabases = map[string]struct{}{
		"mysql": {}, "information_schema": {}, "performance_schema": {}, "sys": {},
	}
	reservedMySQLUsers = map[string]struct{}{
		"root": {}, "mysql": {},
	}
)

const MaxMySQLDump = 32 << 20

func MySQLDump(sql string) error {
	if len(strings.TrimSpace(sql)) == 0 || len(sql) > MaxMySQLDump {
		return fmt.Errorf("SQL file must be between 1 byte and 32 MB")
	}
	if strings.ContainsRune(sql, 0) {
		return fmt.Errorf("SQL file contains invalid characters")
	}
	return nil
}

var firewallCommentRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,31}$`)

func FirewallPort(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid firewall port")
	}
	return nil
}

func FirewallRule(port int, proto, from, comment string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid firewall port")
	}
	switch proto {
	case "tcp", "udp", "any":
	default:
		return fmt.Errorf("firewall protocol must be tcp, udp, or any")
	}
	if from != "any" {
		if ip := net.ParseIP(from); ip == nil {
			if _, _, err := net.ParseCIDR(from); err != nil {
				return fmt.Errorf("firewall source must be any, an IP address, or a network")
			}
		}
	}
	if comment != "" && comment != "-" && !firewallCommentRe.MatchString(comment) {
		return fmt.Errorf("firewall comment must be letters, numbers, spaces, dots, or dashes")
	}
	return nil
}

func FirewallAccess(action, direction string, port int, proto, from, comment string, sshPort int) error {
	if err := FirewallRule(port, proto, from, comment); err != nil {
		return err
	}
	switch action {
	case "allow", "deny", "reject", "limit":
	default:
		return fmt.Errorf("firewall action must be allow, deny, reject, or limit")
	}
	switch direction {
	case "in", "out":
	default:
		return fmt.Errorf("firewall direction must be incoming or outgoing")
	}
	if (action == "allow" || action == "limit") && (port == 3306 || port == 33060) {
		return fmt.Errorf("MySQL stays on this server and is reached through SSH")
	}
	if direction == "in" && port == sshPort && firewallFromAnywhere(from) && action != "allow" {
		return fmt.Errorf("SSH on port %d must stay allowed", sshPort)
	}
	return nil
}

func FirewallPolicy(direction, policy string) error {
	switch direction {
	case "incoming", "outgoing":
	default:
		return fmt.Errorf("invalid firewall direction")
	}
	switch policy {
	case "allow", "deny", "reject":
		return nil
	default:
		return fmt.Errorf("firewall policy must be allow, deny, or reject")
	}
}

func FirewallLogging(level string) error {
	switch level {
	case "off", "low", "medium", "high", "full":
		return nil
	default:
		return fmt.Errorf("firewall logging must be off, low, medium, high, or full")
	}
}

func firewallFromAnywhere(from string) bool {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "", "any", "anywhere", "0.0.0.0/0", "0.0.0.0":
		return true
	default:
		return false
	}
}

func MySQLDatabase(name string) error {
	if _, reserved := reservedMySQLDatabases[strings.ToLower(name)]; reserved || !mysqlIdentRe.MatchString(name) {
		return fmt.Errorf("invalid mysql database name")
	}
	return nil
}

func MySQLUser(name string) error {
	if _, reserved := reservedMySQLUsers[strings.ToLower(name)]; reserved || !mysqlUserRe.MatchString(name) {
		return fmt.Errorf("invalid mysql username")
	}
	return nil
}

func MySQLHost(host string) error {
	switch host {
	case "localhost", "127.0.0.1", "%":
		return nil
	default:
		return fmt.Errorf("mysql host must be localhost, 127.0.0.1, or %%")
	}
}

func MySQLAccount(name, host string) error {
	if err := MySQLUser(name); err != nil {
		return err
	}
	return MySQLHost(host)
}

func MySQLPrivileges(privs []string) error {
	if len(privs) == 0 || len(privs) > len(mysqlPrivs) {
		return fmt.Errorf("choose at least one mysql privilege")
	}
	seen := map[string]struct{}{}
	for _, priv := range privs {
		if _, ok := mysqlPrivs[priv]; !ok {
			return fmt.Errorf("invalid mysql privilege")
		}
		if _, ok := seen[priv]; ok {
			return fmt.Errorf("invalid mysql privilege")
		}
		seen[priv] = struct{}{}
	}
	if _, all := seen["ALL"]; all && len(privs) != 1 {
		return fmt.Errorf("ALL cannot be combined with other privileges")
	}
	return nil
}

func ServerLogin(name, host string, port int, user, password string) error {
	name = strings.TrimSpace(name)
	host = strings.TrimSpace(host)
	user = strings.TrimSpace(user)
	if len(name) < 1 || len(name) > 40 || strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("server name must be 1 to 40 characters")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,253}[A-Za-z0-9])?$`).MatchString(host) {
		return fmt.Errorf("enter the server IP or hostname")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("ssh port must be between 1 and 65535")
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]{0,31}$`).MatchString(user) {
		return fmt.Errorf("invalid ssh username")
	}
	if password != "" && (len(password) > 128 || strings.ContainsAny(password, "\r\n\x00")) {
		return fmt.Errorf("ssh password must be 1 to 128 characters")
	}
	return nil
}

var (
	gitNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._'()-]{0,78}$`)
	gitDirRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	gitSegRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,80}$`)
)

func GitName(value string) error {
	if !gitNameRe.MatchString(value) || strings.ContainsAny(value, "\n\r\x00") {
		return fmt.Errorf("invalid git name")
	}
	return nil
}

func GitDest(path string) error {
	if err := GitRepoPath(path); err != nil {
		return err
	}
	if !gitDirRe.MatchString(filepath.Base(path)) {
		return fmt.Errorf("invalid directory")
	}
	return nil
}

func GitRepoPath(path string) error {
	if path == "" || path != filepath.Clean(path) || strings.Contains(path, "..") {
		return fmt.Errorf("invalid directory")
	}
	for _, root := range []string{"/var/www/", "/opt/", "/srv/", "/root/", "/home/"} {
		if strings.HasPrefix(path, root) && len(path) > len(root) {
			return nil
		}
	}
	return fmt.Errorf("use a directory under /var/www, /opt, /srv, /root, or /home")
}

func GitURL(raw string) error {
	if raw == "" || len(raw) > 300 || strings.ContainsAny(raw, " \t\n\r\x00?#\\'\"") || strings.Contains(raw, "..") {
		return fmt.Errorf("invalid git address")
	}
	raw = strings.TrimSuffix(raw, "/")
	switch {
	case strings.HasPrefix(raw, "https://"), strings.HasPrefix(raw, "http://"):
		rest := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
		if strings.Contains(rest, "@") {
			return fmt.Errorf("git address cannot include a password")
		}
		return gitHTTP(rest)
	case strings.HasPrefix(raw, "ssh://git@"):
		return gitHTTP(strings.TrimPrefix(raw, "ssh://git@"))
	case strings.HasPrefix(raw, "git@"):
		host, path, ok := strings.Cut(strings.TrimPrefix(raw, "git@"), ":")
		if !ok || strings.HasPrefix(path, "/") || strings.Contains(path, ":") {
			return fmt.Errorf("invalid git address")
		}
		if err := gitHost(host); err != nil {
			return err
		}
		return gitPath(path)
	default:
		return fmt.Errorf("invalid git address")
	}
}

func gitHTTP(rest string) error {
	host, path, ok := strings.Cut(rest, "/")
	if !ok || path == "" {
		return fmt.Errorf("invalid git address")
	}
	if err := gitHost(host); err != nil {
		return err
	}
	return gitPath(path)
}

func gitHost(host string) error {
	name := host
	if h, port, ok := strings.Cut(host, ":"); ok {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strings.Contains(h, ":") {
			return fmt.Errorf("invalid git address")
		}
		name = h
	}
	if name == "" || !domainRe.MatchString(name) {
		return fmt.Errorf("invalid git address")
	}
	return nil
}

func gitPath(path string) error {
	path = strings.TrimSuffix(path, ".git")
	if path == "" {
		return fmt.Errorf("invalid git address")
	}
	for _, part := range strings.Split(path, "/") {
		if !gitSegRe.MatchString(part) {
			return fmt.Errorf("invalid git address")
		}
	}
	return nil
}

func MySQLSettings(maxConn, wait string) (int, int, error) {
	connections, err := strconv.Atoi(maxConn)
	if err != nil || connections < 1 || connections > 5000 {
		return 0, 0, fmt.Errorf("max connections must be between 1 and 5000")
	}
	timeout, err := strconv.Atoi(wait)
	if err != nil || timeout < 30 || timeout > 86400 {
		return 0, 0, fmt.Errorf("wait timeout must be between 30 and 86400 seconds")
	}
	return connections, timeout, nil
}
