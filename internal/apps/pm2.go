package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"n-manager/internal/validate"
)

type PM2Status struct {
	Installed bool         `json:"installed"`
	Version   string       `json:"version"`
	Path      string       `json:"path"`
	Processes []PM2Process `json:"processes"`
}

type PM2Process struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	Memory   uint64  `json:"memory"`
	Restarts int     `json:"restarts"`
	Uptime   int64   `json:"uptime"`
	PID      int     `json:"pid"`
}

type pm2Dump []struct {
	Name string `json:"name"`
	PID  int    `json:"pid"`
	Env  struct {
		ID       int    `json:"pm_id"`
		Status   string `json:"status"`
		Uptime   int64  `json:"pm_uptime"`
		Restarts int    `json:"restart_time"`
	} `json:"pm2_env"`
	Monit struct {
		Memory uint64  `json:"memory"`
		CPU    float64 `json:"cpu"`
	} `json:"monit"`
}

func PM2(ctx context.Context) PM2Status {
	st := PM2Status{Processes: []PM2Process{}}
	bin, home := nodeBinDir()
	if bin == "" {
		return st
	}
	pm2 := filepath.Join(bin, "pm2")
	if info, err := os.Stat(pm2); err != nil || info.IsDir() {
		return st
	}
	st.Installed = true
	st.Path = pm2
	if version, err := runBinResult(ctx, bin, home, "pm2", "-v"); err == nil {
		st.Version = pm2Version(version)
	}
	if raw, err := runBinResult(ctx, bin, home, "pm2", "jlist"); err == nil {
		st.Processes = parsePM2(raw)
	}
	return st
}

func ManagedNVM() bool {
	dir, _ := locateNVM()
	return dir == NVMDir
}

func PM2Run(ctx context.Context, action, name, script string) (string, error) {
	if err := validate.PM2Operation(pm2Args(action, name, script)); err != nil {
		return "", err
	}
	bin, home := nodeBinDir()
	if bin == "" {
		return "", fmt.Errorf("node is not installed")
	}
	if action == "install" {
		out, err := runBinResult(ctx, bin, home, "npm", "install", "-g", "pm2")
		return trimOutput(out), err
	}
	pm2 := filepath.Join(bin, "pm2")
	if _, err := os.Stat(pm2); err != nil {
		return "", fmt.Errorf("pm2 is not installed")
	}
	out, err := runBinResult(ctx, bin, home, "pm2", pm2Command(action, name, script)...)
	return trimOutput(out), err
}

func parsePM2(raw string) []PM2Process {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end < start {
		return []PM2Process{}
	}
	var dump pm2Dump
	if err := json.Unmarshal([]byte(raw[start:end+1]), &dump); err != nil {
		return []PM2Process{}
	}
	now := time.Now().UnixMilli()
	out := make([]PM2Process, 0, len(dump))
	for _, item := range dump {
		uptime := int64(0)
		if item.Env.Status == "online" && item.Env.Uptime > 0 && now >= item.Env.Uptime {
			uptime = (now - item.Env.Uptime) / 1000
		}
		out = append(out, PM2Process{
			ID:       item.Env.ID,
			Name:     item.Name,
			Status:   item.Env.Status,
			CPU:      item.Monit.CPU,
			Memory:   item.Monit.Memory,
			Restarts: item.Env.Restarts,
			Uptime:   uptime,
			PID:      item.PID,
		})
	}
	return out
}

func nodeBinDir() (bin, home string) {
	dir, home := locateNVM()
	if _, err := os.Stat(filepath.Join(dir, "nvm.sh")); err != nil {
		return "", home
	}
	versions := installedVersions(dir)
	def := matchInstalled(resolveDefault(filepath.Join(dir, "alias"), 5), versions)
	return nodeBin(dir, def), home
}

func installedVersions(nvmDir string) []string {
	entries, err := os.ReadDir(filepath.Join(nvmDir, "versions", "node"))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	return out
}

func pm2Args(action, name, script string) []string {
	switch action {
	case "install":
		return []string{"install"}
	case "start":
		if script != "" {
			return []string{"start", name, script}
		}
		return []string{"start", name}
	default:
		return []string{action, name}
	}
}

func pm2Command(action, name, script string) []string {
	switch action {
	case "logs":
		return []string{"logs", name, "--nostream", "--lines", "100", "--raw"}
	case "start":
		if script != "" {
			return []string{"start", script, "--name", name}
		}
		return []string{"start", name}
	default:
		return []string{action, name}
	}
}

func pm2Version(text string) string {
	version := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 0 && line[0] >= '0' && line[0] <= '9' && strings.Count(line, ".") >= 1 {
			version = line
		}
	}
	return version
}

func trimOutput(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 8000 {
		return text[len(text)-8000:]
	}
	return text
}

func runBinResult(ctx context.Context, bin, home, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(bin, name), args...)
	env := append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	if home != "" {
		env = append(env, "HOME="+home)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}
