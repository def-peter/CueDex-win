package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserDataSitsUnderAppData(t *testing.T) {
	t.Setenv("AppData", filepath.Join(t.TempDir(), "Roaming"))
	dir, err := UserData()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dir.Root, filepath.Join(os.Getenv("AppData"), "CueDex"); got != want {
		t.Fatalf("root %q, want %q", got, want)
	}
	if got, want := dir.Events(), filepath.Join(dir.Root, "Events"); got != want {
		t.Fatalf("events %q, want %q", got, want)
	}
}

func TestPrepareCreatesBothDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CueDex")
	d := Dir{Root: root}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{root, d.Events()} {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			t.Fatalf("%q not created: %v", p, err)
		}
	}
}
