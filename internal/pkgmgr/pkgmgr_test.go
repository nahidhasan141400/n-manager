package pkgmgr

import "testing"

func TestParse(t *testing.T) {
	mgr, err := Parse("ID=ubuntu\nID_LIKE=debian\nNAME=\"Ubuntu\"\nVERSION=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04\"\n")
	if err != nil || mgr.ID != "apt" {
		t.Fatalf("%+v %v", mgr, err)
	}
	mgr, err = Parse("ID=fedora\nNAME=\"Fedora Linux\"\nVERSION=\"41\"\n")
	if err != nil || mgr.ID != "dnf" {
		t.Fatalf("%+v %v", mgr, err)
	}
	if _, err := Parse("ID=arch\nPRETTY_NAME=\"Arch\"\n"); err == nil {
		t.Fatal("expected unsupported distro")
	}
}
