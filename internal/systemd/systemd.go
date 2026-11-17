package systemd

import (
	"context"
	"strconv"
	"strings"
	"time"

	"n-manager/internal/priv"
	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

type Status struct {
	Unit    string `json:"unit"`
	Active  string `json:"active"`
	Enabled string `json:"enabled"`
	Exists  bool   `json:"exists"`
}

func Probe(ctx context.Context, candidates []string) Status {
	for _, unit := range candidates {
		if !validate.AllowedUnit(unit) {
			continue
		}
		load := strings.TrimSpace(output(ctx, "systemctl", "show", unit, "-p", "LoadState", "--value"))
		if load == "" || load == "not-found" {
			continue
		}
		active := strings.TrimSpace(output(ctx, "systemctl", "show", unit, "-p", "ActiveState", "--value"))
		enabled := strings.TrimSpace(output(ctx, "systemctl", "is-enabled", unit))
		if enabled == "" {
			enabled = "unknown"
		}
		return Status{Unit: unit, Active: active, Enabled: firstWord(enabled), Exists: true}
	}
	if len(candidates) == 0 {
		return Status{}
	}
	return Status{Unit: candidates[0], Active: "not-found", Enabled: "unknown", Exists: false}
}

func Action(ctx context.Context, ex priv.Executor, action, unit string) (string, error) {
	if err := validate.Operation("systemctl", []string{action, unit}); err != nil {
		return "", err
	}
	return ex.Run(ctx, "systemctl", []string{action, unit}, "")
}

func Logs(ctx context.Context, ex priv.Executor, unit string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	args := []string{unit, strconv.Itoa(lines)}
	if err := validate.Operation("journal", args); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := remote.Run(ctx, "journalctl", []string{"-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso"}, "")
	if err == nil {
		return out, nil
	}
	fallback, ferr := ex.Run(ctx, "journal", args, "")
	if ferr != nil {
		return out, err
	}
	return fallback, nil
}

func output(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, _ := remote.Run(ctx, name, args, "")
	return out
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "unknown"
	}
	return fields[0]
}
