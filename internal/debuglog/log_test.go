package debuglog

import (
	"database/sql"
	"path/filepath"
	"testing"

	"n-manager/internal/db"
)

func TestLogRing(t *testing.T) {
	database := openDB(t)
	log := New(database)
	log.Note("error", "lab", "command", "git clone", "host key")
	log.Note("error", "lab", "command", "git clone", "host key")
	log.Note("info", "lab", "ssh", "connected", "root@10.0.0.8:22")
	entries, err := log.Recent(10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries %d", len(entries))
	}
	if entries[0].Message != "connected" || entries[1].Kind != "command" {
		t.Fatalf("%+v", entries)
	}
	errorsOnly, err := log.Recent(10, "error")
	if err != nil {
		t.Fatal(err)
	}
	if len(errorsOnly) != 1 || errorsOnly[0].Message != "git clone" {
		t.Fatalf("%+v", errorsOnly)
	}
	for i := 0; i < keep+5; i++ {
		log.Note("info", "lab", "ssh", "connected", string(rune('a'+(i%26))))
	}
	all, err := log.Recent(keep, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != keep {
		t.Fatalf("kept %d", len(all))
	}
	if err := log.Clear(); err != nil {
		t.Fatal(err)
	}
	left, err := log.Recent(10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("left %d", len(left))
	}
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}
