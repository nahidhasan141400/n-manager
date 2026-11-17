package apps

import (
	"context"
	"os"
	"testing"

	"n-manager/internal/remote"
)

type homeSys struct {
	remote.Missing
	home  string
	files map[string]bool
}

func (s homeSys) Run(_ context.Context, name string, args []string, _ string) (string, error) {
	if name == "bash" && len(args) > 0 && args[0] == "-c" {
		return s.home, nil
	}
	return "", os.ErrNotExist
}

func (s homeSys) Stat(_ context.Context, path string) (os.FileInfo, error) {
	if s.files[path] {
		return remote.NewStat(path, false, 0o644, 1), nil
	}
	return nil, os.ErrNotExist
}

func TestRemoteNVMIgnoresPanelDirectory(t *testing.T) {
	ctx := remote.With(context.Background(), homeSys{
		home:  "/root",
		files: map[string]bool{"/var/lib/n-manager/nvm/nvm.sh": true},
	})
	st := Node(ctx)
	if st.NVMInstalled {
		t.Fatal("panel directory was treated as the server nvm")
	}
	if st.NVMPath != "/root/.nvm" {
		t.Fatalf("path %s", st.NVMPath)
	}
}

func TestMatchInstalledPartialAlias(t *testing.T) {
	versions := []string{"v22.0.0", "v22.21.0", "v24.15.0", "v25.0.0", "v25.9.0"}
	got := matchInstalled("25", versions)
	if got != "v25.9.0" {
		t.Fatalf("alias 25 resolved to %q", got)
	}
	if matchInstalled("v22.0.0", versions) != "v22.0.0" {
		t.Fatal("exact version was not kept")
	}
	if matchInstalled("24", versions) != "v24.15.0" {
		t.Fatal("minor prefix did not match")
	}
}

func TestVersionLess(t *testing.T) {
	if !versionLess("v25.0.0", "v25.9.0") {
		t.Fatal("expected v25.0.0 before v25.9.0")
	}
	if versionLess("v25.9.0", "v22.21.0") {
		t.Fatal("expected v25.9.0 after v22")
	}
}
