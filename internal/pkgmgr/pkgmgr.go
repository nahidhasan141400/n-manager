package pkgmgr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Manager struct {
	ID      string `json:"id"`
	Distro  string `json:"distro"`
	Version string `json:"version"`
}

func Detect() (Manager, error) {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return Manager{}, fmt.Errorf("read os-release: %w", err)
	}
	return Parse(string(b))
}

func Parse(text string) (Manager, error) {
	vals := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[k] = strings.Trim(v, `"`)
	}
	id := strings.ToLower(vals["ID"])
	like := strings.ToLower(vals["ID_LIKE"])
	blob := id + " " + like
	mgr := Manager{Distro: vals["NAME"], Version: vals["VERSION"]}
	switch {
	case containsAny(blob, "debian", "ubuntu", "linuxmint", "pop"):
		mgr.ID = "apt"
	case containsAny(blob, "fedora", "rhel", "centos", "rocky", "alma", "amzn"):
		mgr.ID = "dnf"
	default:
		return mgr, fmt.Errorf("unsupported distribution %q", vals["PRETTY_NAME"])
	}
	return mgr, nil
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func Installed(mgr, pkg string) (bool, string, error) {
	switch mgr {
	case "apt":
		out, err := exec.Command("dpkg-query", "-W", "-f", "${Status}\t${Version}", pkg).CombinedOutput()
		if err != nil {
			return false, "", nil
		}
		status, version, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
		if !strings.Contains(status, "install ok installed") {
			return false, "", nil
		}
		return true, version, nil
	case "dnf":
		out, err := exec.Command("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", pkg).CombinedOutput()
		if err != nil {
			return false, "", nil
		}
		return true, strings.TrimSpace(string(out)), nil
	default:
		return false, "", fmt.Errorf("unsupported package manager")
	}
}

func Available(mgr, pkg string) bool {
	switch mgr {
	case "apt":
		return exec.Command("apt-cache", "show", pkg).Run() == nil
	case "dnf":
		return exec.Command("dnf", "info", "-q", pkg).Run() == nil
	default:
		return false
	}
}
