package apps

import "testing"

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
