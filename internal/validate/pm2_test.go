package validate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPM2Script(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.js")
	if err := os.WriteFile(file, []byte("console.log(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PM2Name("api"); err != nil {
		t.Fatal(err)
	}
	if err := PM2Name("-bad"); err == nil {
		t.Fatal("accepted a bad name")
	}
	if err := checkPM2Script(file, []string{dir}); err != nil {
		t.Fatal(err)
	}
	if err := checkPM2Script(filepath.Join(dir, "..", "app.js"), []string{dir}); err == nil {
		t.Fatal("accepted a path with ..")
	}
	if err := checkPM2Script(filepath.Join(dir, "app.sh"), []string{dir}); err == nil {
		t.Fatal("accepted a shell script")
	}
}
