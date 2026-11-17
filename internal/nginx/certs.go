package nginx

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CertDir = "/etc/nginx/n-manager/certs"

type Certificate struct {
	Name     string `json:"name"`
	Cert     string `json:"cert"`
	Key      string `json:"key"`
	Subject  string `json:"subject"`
	Issuer   string `json:"issuer"`
	NotAfter string `json:"not_after"`
	Days     int    `json:"days"`
	Managed  bool   `json:"managed"`
}

func Certificates() []Certificate {
	var out []Certificate
	out = append(out, managedCerts()...)
	out = append(out, letsEncrypt()...)
	if out == nil {
		return []Certificate{}
	}
	return out
}

func managedCerts() []Certificate {
	entries, err := os.ReadDir(CertDir)
	if err != nil {
		return nil
	}
	var out []Certificate
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".crt") {
			continue
		}
		id := strings.TrimSuffix(name, ".crt")
		certPath := filepath.Join(CertDir, name)
		keyPath := filepath.Join(CertDir, id+".key")
		item, ok := describe(id, certPath, keyPath, true)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func letsEncrypt() []Certificate {
	const root = "/etc/letsencrypt/live"
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Certificate
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		item, ok := describe(entry.Name(), filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem"), false)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func describe(name, certPath, keyPath string, managed bool) (Certificate, bool) {
	info, err := os.Lstat(certPath)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Certificate{}, false
	}
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return Certificate{}, false
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return Certificate{}, false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Certificate{}, false
	}
	days := int(time.Until(cert.NotAfter).Hours() / 24)
	return Certificate{
		Name: name, Cert: certPath, Key: keyPath, Managed: managed,
		Subject: cert.Subject.String(), Issuer: cert.Issuer.String(),
		NotAfter: cert.NotAfter.UTC().Format(time.RFC3339), Days: days,
	}, true
}
