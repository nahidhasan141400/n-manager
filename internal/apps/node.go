package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"n-manager/internal/priv"
	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

const (
	NVMDir     = "/var/lib/n-manager/nvm"
	DeployHome = "/var/lib/n-manager/deploy"
)

type NodeStatus struct {
	NVMInstalled   bool     `json:"nvm_installed"`
	NVMPath        string   `json:"nvm_path"`
	NodeVersion    string   `json:"node_version"`
	NPMVersion     string   `json:"npm_version"`
	Default        string   `json:"default"`
	Versions       []string `json:"versions"`
	GlobalPackages []string `json:"global_packages"`
}

type npmList struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

var versionRe = regexp.MustCompile(`^\d+(?:\.\d+){0,2}$`)

func Node(ctx context.Context) NodeStatus {
	dir, home := locateNVM(ctx)
	st := NodeStatus{NVMPath: dir, Versions: []string{}, GlobalPackages: []string{}}
	if dir == "" {
		st.NVMPath = NVMDir
		return st
	}
	if _, err := remote.Stat(ctx, filepath.Join(dir, "nvm.sh")); err != nil {
		return st
	}
	st.NVMInstalled = true
	entries, err := remote.ReadDir(ctx, filepath.Join(dir, "versions", "node"))
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				st.Versions = append(st.Versions, entry.Name())
			}
		}
		sort.Slice(st.Versions, func(i, j int) bool { return versionLess(st.Versions[i], st.Versions[j]) })
	}
	alias := resolveDefault(ctx, filepath.Join(dir, "alias"), 5)
	st.Default = matchInstalled(alias, st.Versions)
	bin := nodeBin(ctx, dir, st.Default)
	if bin != "" {
		st.NodeVersion = strings.TrimSpace(runBin(ctx, bin, home, "node", "-v"))
		st.NPMVersion = strings.TrimSpace(runBin(ctx, bin, home, "npm", "-v"))
		st.GlobalPackages = globalPackages(ctx, bin, home)
	}
	return st
}

// locateNVM uses the panel directory on this computer. On a server it uses that user's ~/.nvm.
func locateNVM(ctx context.Context) (dir, home string) {
	if _, ok := remote.From(ctx).(remote.Local); ok {
		if _, err := remote.Stat(ctx, filepath.Join(NVMDir, "nvm.sh")); err == nil {
			return NVMDir, DeployHome
		}
		if env := os.Getenv("NVM_DIR"); env != "" {
			if _, err := remote.Stat(ctx, filepath.Join(env, "nvm.sh")); err == nil {
				home, _ = os.UserHomeDir()
				return env, home
			}
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			userNVM := filepath.Join(home, ".nvm")
			if _, err := remote.Stat(ctx, filepath.Join(userNVM, "nvm.sh")); err == nil {
				return userNVM, home
			}
		}
		return NVMDir, DeployHome
	}
	home, err := remoteHome(ctx)
	if err != nil {
		return "", ""
	}
	return filepath.Join(home, ".nvm"), home
}

func remoteHome(ctx context.Context) (string, error) {
	out, err := remote.Run(ctx, "bash", []string{"-c", `printf %s "$HOME"`}, "")
	home := strings.TrimSpace(out)
	if err != nil || !strings.HasPrefix(home, "/") || strings.Contains(home, "..") || strings.ContainsAny(home, "\n\r\x00") {
		return "", fmt.Errorf("the server home directory could not be read")
	}
	return home, nil
}

func InstallNVM(ctx context.Context, ex priv.Executor) (string, error) {
	return ex.Run(ctx, "nvm-install", nil, "")
}

func InstallNode(ctx context.Context, ex priv.Executor, version string) (string, error) {
	if err := validate.NodeVersion(version); err != nil {
		return "", err
	}
	return ex.Run(ctx, "node-install", []string{version}, "")
}

func RemoveNode(ctx context.Context, ex priv.Executor, version string, confirm bool) (string, error) {
	if !confirm {
		return "", fmt.Errorf("confirmation is required")
	}
	if err := validate.NodeVersion(version); err != nil {
		return "", err
	}
	return ex.Run(ctx, "node-uninstall", []string{version}, "")
}

func SetDefault(ctx context.Context, ex priv.Executor, version string) (string, error) {
	if err := validate.NodeVersion(version); err != nil {
		return "", err
	}
	return ex.Run(ctx, "node-default", []string{version}, "")
}

func InstallGlobal(ctx context.Context, ex priv.Executor, name string) (string, error) {
	if err := validate.NPMPackage(name); err != nil {
		return "", err
	}
	return ex.Run(ctx, "npm-global-install", []string{name}, "")
}

func RemoveGlobal(ctx context.Context, ex priv.Executor, name string, confirm bool) (string, error) {
	if !confirm {
		return "", fmt.Errorf("confirmation is required")
	}
	if err := validate.NPMPackage(name); err != nil {
		return "", err
	}
	return ex.Run(ctx, "npm-global-uninstall", []string{name}, "")
}

func resolveDefault(ctx context.Context, aliasDir string, depth int) string {
	cur := "default"
	for i := 0; i < depth; i++ {
		b, err := remote.ReadFile(ctx, filepath.Join(aliasDir, cur))
		if err != nil {
			if i == 0 {
				return ""
			}
			return cur
		}
		next := strings.TrimSpace(string(b))
		if next == "" || next == cur {
			return cur
		}
		if strings.HasPrefix(next, "v") {
			return next
		}
		if versionRe.MatchString(next) {
			return "v" + next
		}
		if _, err := remote.Stat(ctx, filepath.Join(aliasDir, next)); err != nil {
			return next
		}
		cur = next
	}
	return cur
}

func nodeBin(ctx context.Context, nvmDir, version string) string {
	if version == "" {
		return ""
	}
	dir := filepath.Join(nvmDir, "versions", "node", version, "bin")
	if st, err := remote.Stat(ctx, filepath.Join(dir, "node")); err == nil && !st.IsDir() {
		return dir
	}
	return ""
}

func matchInstalled(spec string, versions []string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" || len(versions) == 0 {
		return ""
	}
	want := spec
	if !strings.HasPrefix(want, "v") && versionRe.MatchString(want) {
		want = "v" + want
	}
	for _, version := range versions {
		if version == spec || version == want {
			return version
		}
	}
	best := ""
	for _, version := range versions {
		if version == want || strings.HasPrefix(version, want+".") {
			if best == "" || versionLess(best, version) {
				best = version
			}
		}
	}
	return best
}

func versionLess(a, b string) bool {
	ap := versionParts(a)
	bp := versionParts(b)
	for i := 0; i < len(ap) || i < len(bp); i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av != bv {
			return av < bv
		}
	}
	return false
}

func versionParts(value string) []int {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	fields := strings.Split(value, ".")
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		n := 0
		for _, ch := range field {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		out = append(out, n)
	}
	return out
}

func runBin(ctx context.Context, bin, home, name string, args ...string) string {
	out, err := runBinResult(ctx, bin, home, name, args...)
	if err != nil {
		return ""
	}
	return out
}

func globalPackages(ctx context.Context, bin, home string) []string {
	raw := runBin(ctx, bin, home, "npm", "ls", "-g", "--depth=0", "--json")
	if raw == "" {
		return []string{}
	}
	var parsed npmList
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(parsed.Dependencies))
	for name, dep := range parsed.Dependencies {
		out = append(out, name+"@"+dep.Version)
	}
	sort.Strings(out)
	return out
}
