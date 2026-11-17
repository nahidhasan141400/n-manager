package db

import (
	"path/filepath"
	"testing"
)

func TestMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "app.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st, err := StatusOf(database, path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Ready || st.Version != LatestVersion {
		t.Fatalf("status %+v", st)
	}
	database.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	version, err := CurrentVersion(again)
	if err != nil {
		t.Fatal(err)
	}
	if version != LatestVersion {
		t.Fatalf("version %d", version)
	}
}
