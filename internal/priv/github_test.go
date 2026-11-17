package priv

import (
	"strings"
	"testing"
)

func TestGitHubLogin(t *testing.T) {
	text := "Hi octocat! You've successfully authenticated, but GitHub does not provide shell access."
	login, ok := githubLogin(text)
	if !ok || login != "octocat" {
		t.Fatalf("login %q %v", login, ok)
	}
	if _, ok := githubLogin("Permission denied (publickey)."); ok {
		t.Fatal("accepted a rejected key")
	}
}

func TestMergeGitHubConfig(t *testing.T) {
	block := githubConfig("/root/.ssh/nmanager_github")
	first := mergeGitHubConfig("Host other\n", block)
	if !strings.Contains(first, "IdentityFile /root/.ssh/nmanager_github") || !strings.Contains(first, "Host other") {
		t.Fatalf("merged %q", first)
	}
	again := mergeGitHubConfig(first, githubConfig("/root/.ssh/nmanager_github"))
	if strings.Count(again, "# n-manager github") != 1 {
		t.Fatalf("duplicated block %q", again)
	}
}

func TestGitHubHostKeys(t *testing.T) {
	got := githubHostKeys("# github.com:22 SSH-2.0\ngithub.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI\nbad.example ssh-ed25519 AAAA\n")
	if got != "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI\n" {
		t.Fatalf("keys %q", got)
	}
}
