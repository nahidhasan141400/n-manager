package apps

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"n-manager/internal/pkgmgr"
	"n-manager/internal/priv"
	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

type GitStatus struct {
	Installed bool       `json:"installed"`
	Version   string     `json:"version"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Repos     []GitRepo  `json:"repos"`
	GitHub    GitHubLink `json:"github"`
}

type GitHubLink struct {
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
	Login     string `json:"login"`
	PublicKey string `json:"public_key"`
}

type GitRepo struct {
	Path   string `json:"path"`
	Remote string `json:"remote"`
	Branch string `json:"branch"`
}

var gitRoots = []struct {
	path  string
	depth int
}{
	{"/var/www", 3},
	{"/opt", 2},
	{"/srv", 2},
	{"/root", 2},
	{"/home", 3},
}

func Git(ctx context.Context) GitStatus {
	st := GitStatus{Repos: []GitRepo{}, GitHub: githubStatus(ctx)}
	out, err := remote.Run(ctx, "git", []string{"--version"}, "")
	if err != nil {
		return st
	}
	st.Installed = true
	st.Version = gitVersion(out)
	if name, err := remote.Run(ctx, "git", []string{"config", "--global", "--get", "user.name"}, ""); err == nil {
		st.Name = strings.TrimSpace(name)
	}
	if email, err := remote.Run(ctx, "git", []string{"config", "--global", "--get", "user.email"}, ""); err == nil {
		st.Email = strings.TrimSpace(email)
	}
	st.Repos = collectRepos(ctx)
	return st
}

func InstallGit(ctx context.Context, ex priv.Executor) (string, error) {
	mgr, err := pkgmgr.Detect(ctx)
	if err != nil {
		return "", err
	}
	return ex.Run(ctx, "pkg-install", []string{mgr.ID, "git"}, "")
}

func ConfigureGit(ctx context.Context, ex priv.Executor, name, email string) (string, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if err := validate.GitName(name); err != nil {
		return "", err
	}
	if err := validate.Email(email); err != nil {
		return "", err
	}
	return ex.Run(ctx, "git-config", []string{name, email}, "")
}

func CloneGit(ctx context.Context, ex priv.Executor, url, dest string) (string, error) {
	dest = filepath.Clean(strings.TrimSpace(dest))
	url = strings.TrimSpace(url)
	if err := validate.GitURL(url); err != nil {
		return "", err
	}
	if err := validate.GitDest(dest); err != nil {
		return "", err
	}
	if _, err := remote.Stat(ctx, dest); err == nil {
		return "", fmt.Errorf("that directory already exists")
	}
	parent := filepath.Dir(dest)
	info, err := remote.Stat(ctx, parent)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("the parent directory does not exist")
	}
	return ex.Run(ctx, "git-clone", []string{url, dest}, "")
}

func ConnectGitHub(ctx context.Context, ex priv.Executor) (string, error) {
	return ex.Run(ctx, "git-github", nil, "")
}

func TestGitHub(ctx context.Context, ex priv.Executor) (string, error) {
	return ex.Run(ctx, "git-github-test", nil, "")
}

func PullGit(ctx context.Context, ex priv.Executor, path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if err := validate.GitRepoPath(path); err != nil {
		return "", err
	}
	if !repoAt(ctx, path) {
		return "", fmt.Errorf("that directory is not a git repository")
	}
	return ex.Run(ctx, "git-pull", []string{path}, "")
}

func gitVersion(text string) string {
	fields := strings.Fields(text)
	if len(fields) >= 3 && fields[0] == "git" && fields[1] == "version" {
		return fields[2]
	}
	return strings.TrimSpace(text)
}

func collectRepos(ctx context.Context) []GitRepo {
	var out []GitRepo
	seen := 0
	for _, root := range gitRoots {
		walkRepos(ctx, root.path, root.depth, &out, &seen)
		if seen >= 20 {
			break
		}
	}
	if out == nil {
		return []GitRepo{}
	}
	return out
}

func walkRepos(ctx context.Context, dir string, depth int, out *[]GitRepo, seen *int) {
	if *seen >= 20 || depth < 0 {
		return
	}
	if repoAt(ctx, dir) {
		*out = append(*out, describeRepo(ctx, dir))
		*seen++
		return
	}
	if depth == 0 {
		return
	}
	entries, err := remote.ReadDir(ctx, dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		walkRepos(ctx, filepath.Join(dir, entry.Name()), depth-1, out, seen)
		if *seen >= 20 {
			return
		}
	}
}

func repoAt(ctx context.Context, path string) bool {
	_, err := remote.Stat(ctx, filepath.Join(path, ".git"))
	return err == nil
}

func describeRepo(ctx context.Context, path string) GitRepo {
	repo := GitRepo{Path: path}
	if branch, err := remote.Run(ctx, "git", []string{"-C", path, "rev-parse", "--abbrev-ref", "HEAD"}, ""); err == nil {
		repo.Branch = strings.TrimSpace(branch)
	}
	if remoteURL, err := remote.Run(ctx, "git", []string{"-C", path, "config", "--get", "remote.origin.url"}, ""); err == nil {
		repo.Remote = strings.TrimSpace(remoteURL)
	}
	return repo
}
