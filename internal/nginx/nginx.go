package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"n-manager/internal/validate"
)

type Spec struct {
	Name              string   `json:"name"`
	Domains           []string `json:"domains"`
	Mode              string   `json:"mode"`
	ProxyPass         string   `json:"proxy_pass"`
	WebRoot           string   `json:"web_root"`
	SSLCertificate    string   `json:"ssl_certificate"`
	SSLCertificateKey string   `json:"ssl_certificate_key"`
	Redirect          bool     `json:"redirect"`
	BodySize          string   `json:"body_size"`
	Websocket         bool     `json:"websocket"`
	Enabled           bool     `json:"enabled"`
	Config            string   `json:"config,omitempty"`
}

type Site struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Config  string `json:"config"`
	Domains string `json:"domains"`
}

type Layout struct {
	Kind      string
	Available string
	Enabled   string
}

func Detect(root string) (Layout, error) {
	if root == "" {
		root = "/etc/nginx"
	}
	sites := filepath.Join(root, "sites-available")
	if st, err := os.Stat(sites); err == nil && st.IsDir() {
		return Layout{Kind: "sites", Available: sites, Enabled: filepath.Join(root, "sites-enabled")}, nil
	}
	confd := filepath.Join(root, "conf.d")
	if st, err := os.Stat(confd); err == nil && st.IsDir() {
		return Layout{Kind: "conf", Available: confd, Enabled: confd}, nil
	}
	return Layout{}, fmt.Errorf("nginx configuration directory was not found")
}

func Render(spec Spec) (string, error) {
	if err := validate.SiteName(spec.Name); err != nil {
		return "", err
	}
	if len(spec.Domains) == 0 {
		return "", fmt.Errorf("at least one domain is required")
	}
	for _, d := range spec.Domains {
		if err := validate.Domain(strings.TrimSpace(d)); err != nil {
			return "", err
		}
	}
	if err := validate.NginxBodySize(spec.BodySize); err != nil {
		return "", err
	}
	https := spec.SSLCertificate != "" || spec.SSLCertificateKey != ""
	if https {
		if err := validate.CertPath(spec.SSLCertificate); err != nil {
			return "", fmt.Errorf("ssl certificate: %w", err)
		}
		if err := validate.CertPath(spec.SSLCertificateKey); err != nil {
			return "", fmt.Errorf("ssl certificate key: %w", err)
		}
	}
	if spec.Redirect && !https {
		return "", fmt.Errorf("https redirect needs a certificate")
	}
	names := strings.Join(trimAll(spec.Domains), " ")
	var b strings.Builder
	b.WriteString("# managed-by: n-manager\n")
	if spec.Redirect {
		b.WriteString("server {\n")
		b.WriteString("    listen 80;\n")
		fmt.Fprintf(&b, "    server_name %s;\n", names)
		writeChallenge(&b)
		b.WriteString("    location / {\n")
		b.WriteString("        return 301 https://$host$request_uri;\n")
		b.WriteString("    }\n")
		b.WriteString("}\n")
	}
	b.WriteString("server {\n")
	if !spec.Redirect {
		b.WriteString("    listen 80;\n")
	}
	if https {
		b.WriteString("    listen 443 ssl;\n")
		fmt.Fprintf(&b, "    ssl_certificate %s;\n", spec.SSLCertificate)
		fmt.Fprintf(&b, "    ssl_certificate_key %s;\n", spec.SSLCertificateKey)
	}
	fmt.Fprintf(&b, "    server_name %s;\n", names)
	if spec.BodySize != "" {
		fmt.Fprintf(&b, "    client_max_body_size %s;\n", spec.BodySize)
	}
	writeChallenge(&b)
	switch spec.Mode {
	case "proxy":
		if err := validate.ProxyPass(spec.ProxyPass); err != nil {
			return "", err
		}
		b.WriteString("    location / {\n")
		fmt.Fprintf(&b, "        proxy_pass %s;\n", spec.ProxyPass)
		b.WriteString("        proxy_set_header Host $host;\n")
		b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
		if spec.Websocket {
			b.WriteString("        proxy_http_version 1.1;\n")
			b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
			b.WriteString("        proxy_set_header Connection \"upgrade\";\n")
		}
		b.WriteString("    }\n")
	case "root":
		if err := validate.WebRoot(spec.WebRoot); err != nil {
			return "", err
		}
		b.WriteString("    location / {\n")
		fmt.Fprintf(&b, "        root %s;\n", spec.WebRoot)
		b.WriteString("        index index.html;\n")
		b.WriteString("        try_files $uri $uri/ =404;\n")
		b.WriteString("    }\n")
	default:
		return "", fmt.Errorf("mode must be proxy or root")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

const challengeBlock = "    location ^~ /.well-known/acme-challenge/ {\n        root /var/www/letsencrypt;\n        default_type \"text/plain\";\n    }\n"

func writeChallenge(b *strings.Builder) {
	b.WriteString(challengeBlock)
}

func EnsureChallenge(content string) (string, error) {
	if err := validate.NginxContent(content); err != nil {
		return "", err
	}
	if strings.Contains(content, "/.well-known/acme-challenge/") {
		return content, nil
	}
	const location = "    location / {"
	if !strings.Contains(content, location) {
		return "", fmt.Errorf("site config has no location for the certificate challenge")
	}
	return strings.Replace(content, location, challengeBlock+location, 1), nil
}

func UseCertificate(content, cert, key string) (string, error) {
	if err := validate.CertPath(cert); err != nil {
		return "", fmt.Errorf("ssl certificate: %w", err)
	}
	if err := validate.CertPath(key); err != nil {
		return "", fmt.Errorf("ssl certificate key: %w", err)
	}
	content, err := EnsureChallenge(content)
	if err != nil {
		return "", err
	}
	if strings.Contains(content, "ssl_certificate ") {
		content = replaceDirective(content, "ssl_certificate_key", "    ssl_certificate_key "+key+";")
		content = replaceDirective(content, "ssl_certificate", "    ssl_certificate "+cert+";")
	} else {
		const location = "    location / {"
		insert := "    listen 443 ssl;\n    ssl_certificate " + cert + ";\n    ssl_certificate_key " + key + ";\n"
		index := strings.LastIndex(content, location)
		if index < 0 {
			return "", fmt.Errorf("site config has no location for the certificate")
		}
		content = content[:index] + insert + content[index:]
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := validate.NginxContent(content); err != nil {
		return "", err
	}
	return content, nil
}

func replaceDirective(content, directive, replacement string) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, directive) {
			rest := trim[len(directive):]
			if rest == "" || rest[0] == ' ' || rest[0] == '\t' {
				out = append(out, replacement)
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func Prepare(spec Spec) (string, error) {
	if strings.TrimSpace(spec.Config) != "" {
		if err := validate.SiteName(spec.Name); err != nil {
			return "", err
		}
		if err := validate.NginxContent(spec.Config); err != nil {
			return "", err
		}
		if !strings.HasSuffix(spec.Config, "\n") {
			spec.Config += "\n"
		}
		return spec.Config, nil
	}
	return Render(spec)
}

type Tester func() (string, error)

func Apply(layout Layout, name, content string, enabled bool, test Tester) error {
	if err := validate.SiteName(name); err != nil {
		return err
	}
	if err := validate.NginxContent(content); err != nil {
		return err
	}
	if err := os.MkdirAll(layout.Available, 0o755); err != nil {
		return err
	}
	if layout.Kind == "sites" {
		if err := os.MkdirAll(layout.Enabled, 0o755); err != nil {
			return err
		}
	}
	path := filepath.Join(layout.Available, name+".conf")
	prev, hadPrev, err := readOptional(path)
	if err != nil {
		return err
	}
	prevLink, hadLink, err := linkState(layout, name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	if err := setEnabled(layout, name, enabled); err != nil {
		restore(path, prev, hadPrev)
		_ = restoreLink(layout, name, prevLink, hadLink)
		return err
	}
	if _, err := test(); err != nil {
		restore(path, prev, hadPrev)
		_ = restoreLink(layout, name, prevLink, hadLink)
		_, _ = test()
		return fmt.Errorf("nginx -t failed: %w", err)
	}
	return nil
}

func Delete(layout Layout, name string, test Tester) error {
	if err := validate.SiteName(name); err != nil {
		return err
	}
	path := filepath.Join(layout.Available, name+".conf")
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := validate.NginxContent(string(content)); err != nil {
		return err
	}
	prevLink, hadLink, err := linkState(layout, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	_ = removeEnabled(layout, name)
	if _, err := test(); err != nil {
		_ = os.WriteFile(path, content, 0o644)
		_ = restoreLink(layout, name, prevLink, hadLink)
		_, _ = test()
		return fmt.Errorf("nginx -t failed: %w", err)
	}
	return nil
}

func List(layout Layout) ([]Site, error) {
	entries, err := os.ReadDir(layout.Available)
	if err != nil {
		if os.IsNotExist(err) {
			return []Site{}, nil
		}
		return nil, err
	}
	var out []Site
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		disabledFile := strings.HasSuffix(filename, ".conf.disabled")
		if !strings.HasSuffix(filename, ".conf") && !disabledFile {
			continue
		}
		path := filepath.Join(layout.Available, filename)
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "# managed-by: n-manager") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(filename, ".disabled"), ".conf")
		enabled := !disabledFile
		if layout.Kind == "sites" {
			var err error
			enabled, err = isEnabled(layout, name)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, Site{
			Name: name, Enabled: enabled, Path: path, Config: string(b), Domains: serverName(string(b)),
		})
	}
	if out == nil {
		out = []Site{}
	}
	return out, nil
}

func Read(layout Layout, name string) (Site, error) {
	if err := validate.SiteName(name); err != nil {
		return Site{}, err
	}
	path := filepath.Join(layout.Available, name+".conf")
	b, err := os.ReadFile(path)
	if err != nil {
		return Site{}, err
	}
	if err := validate.NginxContent(string(b)); err != nil {
		return Site{}, err
	}
	enabled, err := isEnabled(layout, name)
	if err != nil {
		return Site{}, err
	}
	return Site{Name: name, Enabled: enabled, Path: path, Config: string(b), Domains: serverName(string(b))}, nil
}

func setEnabled(layout Layout, name string, enabled bool) error {
	if !enabled {
		return removeEnabled(layout, name)
	}
	if layout.Kind == "conf" {
		live := filepath.Join(layout.Available, name+".conf")
		disabled := live + ".disabled"
		if _, err := os.Stat(live); err == nil {
			_ = os.Remove(disabled)
			return nil
		}
		if _, err := os.Stat(disabled); err == nil {
			return os.Rename(disabled, live)
		}
		return nil
	}
	link := filepath.Join(layout.Enabled, name+".conf")
	target := filepath.Join(layout.Available, name+".conf")
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, link)
}

func removeEnabled(layout Layout, name string) error {
	if layout.Kind == "conf" {
		live := filepath.Join(layout.Available, name+".conf")
		disabled := live + ".disabled"
		if _, err := os.Stat(live); err == nil {
			return os.Rename(live, disabled)
		}
		return nil
	}
	link := filepath.Join(layout.Enabled, name+".conf")
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func isEnabled(layout Layout, name string) (bool, error) {
	if layout.Kind == "conf" {
		_, err := os.Stat(filepath.Join(layout.Available, name+".conf"))
		if err == nil {
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_, err := os.Stat(filepath.Join(layout.Enabled, name+".conf"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func linkState(layout Layout, name string) (string, bool, error) {
	if layout.Kind != "sites" {
		return "", false, nil
	}
	link := filepath.Join(layout.Enabled, name+".conf")
	target, err := os.Readlink(link)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return target, true, nil
}

func restoreLink(layout Layout, name, target string, had bool) error {
	if layout.Kind != "sites" {
		return nil
	}
	link := filepath.Join(layout.Enabled, name+".conf")
	_ = os.Remove(link)
	if had {
		return os.Symlink(target, link)
	}
	return nil
}

func readOptional(path string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(b), true, nil
}

func restore(path, prev string, had bool) {
	if !had {
		_ = os.Remove(path)
		return
	}
	_ = os.WriteFile(path, []byte(prev), 0o644)
}

func serverName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "server_name") {
			return strings.Trim(strings.TrimPrefix(line, "server_name"), " ;")
		}
	}
	return ""
}

func Syntax(content string) (string, error) {
	if _, err := exec.LookPath("nginx"); err != nil {
		return "", fmt.Errorf("nginx is not installed")
	}
	dir, err := os.MkdirTemp("", "n-manager-nginx-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	site := filepath.Join(dir, "site.conf")
	if err := os.WriteFile(site, []byte(content), 0o644); err != nil {
		return "", err
	}
	wrapper := fmt.Sprintf("worker_processes 1;\nerror_log %s/error.log;\npid %s/nginx.pid;\nevents {}\nhttp {\n    include /etc/nginx/mime.types;\n    include %s;\n}\n", dir, dir, site)
	conf := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(conf, []byte(wrapper), 0o644); err != nil {
		return "", err
	}
	return runCmd("nginx", "-t", "-c", conf)
}

func LiveTest() (string, error) {
	return runCmd("nginx", "-t")
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func Tail(path string, lines int) (string, error) {
	if err := validate.ReadablePath(path); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	readSize := int64(256 * 1024)
	if size < readSize {
		readSize = size
	}
	buf := make([]byte, readSize)
	if readSize > 0 {
		if _, err := f.ReadAt(buf, size-readSize); err != nil {
			return "", err
		}
	}
	parts := strings.Split(string(buf), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n"), nil
}
