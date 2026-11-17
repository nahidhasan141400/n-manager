package apps

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"n-manager/internal/remote"
)

const githubKeyName = "nmanager_github"

func githubStatus(ctx context.Context) GitHubLink {
	home := githubHome(ctx)
	pub, err := remote.ReadFile(ctx, filepath.Join(home, ".ssh", githubKeyName+".pub"))
	if err != nil {
		return GitHubLink{}
	}
	key := strings.TrimSpace(string(pub))
	if !strings.HasPrefix(key, "ssh-ed25519 ") || strings.ContainsAny(key, "\n\r") {
		return GitHubLink{}
	}
	link := GitHubLink{Ready: true, PublicKey: key}
	raw, err := remote.ReadFile(ctx, filepath.Join(home, ".ssh", githubKeyName+".login"))
	if err != nil {
		return link
	}
	login := strings.TrimSpace(string(raw))
	if githubLoginName.MatchString(login) {
		link.Connected = true
		link.Login = login
	}
	return link
}

var githubLoginName = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)

func githubHome(ctx context.Context) string {
	out, err := remote.Run(ctx, "bash", []string{"-c", `printf %s "$HOME"`}, "")
	home := strings.TrimSpace(out)
	if err != nil || !safeGitHome(home) {
		return "/root"
	}
	return home
}

func safeGitHome(home string) bool {
	if home == "" || home != filepath.Clean(home) || strings.Contains(home, "..") || strings.ContainsAny(home, " \t\n\r") {
		return false
	}
	return home == "/root" || strings.HasPrefix(home, "/home/")
}
