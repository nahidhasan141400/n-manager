package priv

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"n-manager/internal/remote"
)

const (
	githubKeyName  = "nmanager_github"
	githubMark     = "# n-manager github"
	githubMarkEnd  = "# end n-manager github"
)

var githubHostLine = regexp.MustCompile(`^github\.com (?:ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp256) [A-Za-z0-9+/=]+$`)
var githubHi = regexp.MustCompile(`Hi ([A-Za-z0-9-]{1,39})! You've successfully authenticated`)

func connectGitHub(ctx context.Context) (string, error) {
	home, key, err := githubPaths(ctx)
	if err != nil {
		return "", err
	}
	if err := ensureGitHubKey(ctx, home, key); err != nil {
		return "", err
	}
	if err := ensureGitHubSSH(ctx, home, key); err != nil {
		return "", err
	}
	login, authErr := probeGitHub(ctx, key)
	if err := rememberGitHub(ctx, home, login); err != nil {
		return "", err
	}
	if authErr != nil {
		_ = clearGitHubInstead(ctx)
		return "Add the public key to GitHub, then test the connection", nil
	}
	_ = preferGitHubSSH(ctx)
	return "Connected as " + login, nil
}

func testGitHub(ctx context.Context) (string, error) {
	home, key, err := githubPaths(ctx)
	if err != nil {
		return "", err
	}
	if _, err := remote.Stat(ctx, key); err != nil {
		return "", fmt.Errorf("create the GitHub key first")
	}
	login, authErr := probeGitHub(ctx, key)
	if err := rememberGitHub(ctx, home, login); err != nil {
		return "", err
	}
	if authErr != nil {
		_ = clearGitHubInstead(ctx)
		return "", authErr
	}
	_ = preferGitHubSSH(ctx)
	return "Connected as " + login, nil
}

func githubPaths(ctx context.Context) (string, string, error) {
	out, err := remote.Run(ctx, "bash", []string{"-c", `printf %s "$HOME"`}, "")
	home := strings.TrimSpace(out)
	if err != nil || home == "" || home != filepath.Clean(home) || strings.Contains(home, "..") || strings.ContainsAny(home, " \t\n\r") || (home != "/root" && !strings.HasPrefix(home, "/home/")) {
		home = "/root"
	}
	return home, filepath.Join(home, ".ssh", githubKeyName), nil
}

func ensureGitHubKey(ctx context.Context, home, key string) error {
	sshDir := filepath.Join(home, ".ssh")
	if err := remote.MkdirAll(ctx, sshDir, 0o700); err != nil {
		return err
	}
	if _, err := remote.Run(ctx, "chmod", []string{"700", sshDir}, ""); err != nil {
		return err
	}
	if _, err := remote.Stat(ctx, key); err == nil {
		return nil
	}
	if _, err := remote.Run(ctx, "ssh-keygen", []string{"-t", "ed25519", "-f", key, "-N", "", "-C", "n-manager"}, ""); err != nil {
		return err
	}
	_, _ = remote.Run(ctx, "chmod", []string{"600", key}, "")
	_, _ = remote.Run(ctx, "chmod", []string{"644", key + ".pub"}, "")
	return nil
}

func ensureGitHubSSH(ctx context.Context, home, key string) error {
	configPath := filepath.Join(home, ".ssh", "config")
	existing := ""
	if raw, err := remote.ReadFile(ctx, configPath); err == nil {
		existing = string(raw)
	}
	next := mergeGitHubConfig(existing, githubConfig(key))
	if err := remote.WriteFile(ctx, configPath, []byte(next), 0o600); err != nil {
		return err
	}
	knownPath := filepath.Join(home, ".ssh", "known_hosts")
	known := ""
	if raw, err := remote.ReadFile(ctx, knownPath); err == nil {
		known = string(raw)
	}
	if strings.Contains(known, "github.com ") {
		return nil
	}
	scanned, err := remote.Run(ctx, "ssh-keyscan", []string{"-t", "ed25519,ecdsa,rsa", "github.com"}, "")
	if err != nil && strings.TrimSpace(scanned) == "" {
		return fmt.Errorf("could not read GitHub's host key")
	}
	added := githubHostKeys(scanned)
	if added == "" {
		return fmt.Errorf("could not read GitHub's host key")
	}
	if known != "" && !strings.HasSuffix(known, "\n") {
		known += "\n"
	}
	return remote.WriteFile(ctx, knownPath, []byte(known+added), 0o644)
}

func probeGitHub(ctx context.Context, key string) (string, error) {
	out, err := remote.Run(ctx, "ssh", []string{
		"-i", key,
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=8",
		"-T", "git@github.com",
	}, "")
	text := out
	if err != nil {
		text = err.Error()
	}
	if login, ok := githubLogin(text); ok {
		return login, nil
	}
	if strings.Contains(text, "Permission denied") {
		return "", fmt.Errorf("GitHub rejected the key. Add the public key on GitHub, then test again")
	}
	if strings.Contains(text, "Host key verification failed") {
		return "", fmt.Errorf("GitHub's host key is not trusted yet")
	}
	return "", fmt.Errorf("could not reach GitHub")
}

func rememberGitHub(ctx context.Context, home, login string) error {
	path := filepath.Join(home, ".ssh", githubKeyName+".login")
	if login == "" {
		_ = remote.Remove(ctx, path)
		return nil
	}
	return remote.WriteFile(ctx, path, []byte(login+"\n"), 0o600)
}

func preferGitHubSSH(ctx context.Context) error {
	_, err := remote.Run(ctx, "git", []string{"config", "--global", "url.git@github.com:.insteadOf", "https://github.com/"}, "")
	return err
}

func clearGitHubInstead(ctx context.Context) error {
	_, err := remote.Run(ctx, "git", []string{"config", "--global", "--unset", "url.git@github.com:.insteadOf"}, "")
	return err
}

func githubConfig(key string) string {
	return githubMark + "\nHost github.com\n  HostName github.com\n  User git\n  IdentityFile " + key + "\n  IdentitiesOnly yes\n" + githubMarkEnd + "\n"
}

func mergeGitHubConfig(existing, block string) string {
	start := strings.Index(existing, githubMark)
	end := strings.Index(existing, githubMarkEnd)
	if start >= 0 && end > start {
		end += len(githubMarkEnd)
		for end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
			end++
		}
		return existing[:start] + block + existing[end:]
	}
	if existing == "" {
		return block
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return block + existing
}

func githubHostKeys(scanned string) string {
	var lines []string
	for _, line := range strings.Split(scanned, "\n") {
		line = strings.TrimSpace(line)
		if githubHostLine.MatchString(line) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func githubLogin(text string) (string, bool) {
	match := githubHi.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	return match[1], true
}
