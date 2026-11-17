package validate

import "testing"

func TestGitURL(t *testing.T) {
	ok := []string{
		"https://github.com/org/app.git",
		"https://github.com/org/app",
		"http://git.example.com/team/app.git",
		"ssh://git@github.com/org/app.git",
		"git@github.com:org/app.git",
		"https://gitlab.com:443/group/app.git",
	}
	for _, item := range ok {
		if err := GitURL(item); err != nil {
			t.Fatalf("%s: %v", item, err)
		}
	}
	bad := []string{
		"https://user:token@github.com/org/app.git",
		"file:///etc/passwd",
		"git@github.com:../../etc/passwd",
		"https://github.com/org/app.git?token=1",
		"ssh://root@github.com/org/app.git",
		"/var/www/app",
	}
	for _, item := range bad {
		if err := GitURL(item); err == nil {
			t.Fatalf("accepted %s", item)
		}
	}
}

func TestGitPaths(t *testing.T) {
	if err := GitDest("/var/www/app"); err != nil {
		t.Fatal(err)
	}
	if err := GitDest("/home/ubuntu/sites/app"); err != nil {
		t.Fatal(err)
	}
	if err := GitDest("/etc/app"); err == nil {
		t.Fatal("accepted /etc")
	}
	if err := GitDest("/var/www/../etc/passwd"); err == nil {
		t.Fatal("accepted traversal")
	}
	if err := GitRepoPath("/opt/app"); err != nil {
		t.Fatal(err)
	}
	if err := Operation("git-clone", []string{"https://github.com/org/app.git", "/var/www/app"}); err != nil {
		t.Fatal(err)
	}
	if err := Operation("git-clone", []string{"https://github.com/org/app.git", "/etc/app"}); err == nil {
		t.Fatal("accepted clone outside the allowed directories")
	}
	if err := Operation("pkg-install", []string{"apt", "git"}); err != nil {
		t.Fatal(err)
	}
}
