package apps

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"n-manager/internal/remote"
)

func TestGitVersion(t *testing.T) {
	if got := gitVersion("git version 2.43.0"); got != "2.43.0" {
		t.Fatalf("version %q", got)
	}
}

func TestCollectRepos(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := remote.With(context.Background(), gitMem{
		Failure: remote.Failure{Err: os.ErrNotExist},
		dirs: map[string][]os.DirEntry{
			root: {remote.NewFile("app", true, os.ModeDir)},
		},
		present: map[string]bool{
			filepath.Join(repo, ".git"): true,
			repo:                        true,
		},
		cmds: map[string]string{
			"git\x00-C\x00" + repo + "\x00rev-parse\x00--abbrev-ref\x00HEAD":    "main",
			"git\x00-C\x00" + repo + "\x00config\x00--get\x00remote.origin.url": "https://github.com/org/app.git",
		},
	})
	var out []GitRepo
	seen := 0
	walkRepos(ctx, root, 2, &out, &seen)
	if len(out) != 1 || out[0].Path != repo || out[0].Branch != "main" || out[0].Remote != "https://github.com/org/app.git" {
		t.Fatalf("repos %#v", out)
	}
}

type gitMem struct {
	remote.Failure
	dirs    map[string][]os.DirEntry
	present map[string]bool
	cmds    map[string]string
}

func (m gitMem) Stat(_ context.Context, path string) (os.FileInfo, error) {
	if m.present[path] {
		return remote.NewFile(filepath.Base(path), true, os.ModeDir), nil
	}
	return nil, os.ErrNotExist
}

func (m gitMem) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	entries, ok := m.dirs[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return entries, nil
}

func (m gitMem) Run(_ context.Context, name string, args []string, _ string) (string, error) {
	key := name
	for _, arg := range args {
		key += "\x00" + arg
	}
	out, ok := m.cmds[key]
	if !ok {
		return "", os.ErrNotExist
	}
	return out, nil
}
