package nvim

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nt", "nvim")
	if present, _ := Installed(dir); present {
		t.Fatal("a plugin is found before the install")
	}
	if err := Install(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"lua/nt/init.lua", "plugin/nt.lua"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if present, current := Installed(dir); !present || !current {
		t.Fatalf("after the install: present %v, current %v", present, current)
	}

	if err := os.WriteFile(filepath.Join(dir, "lua", "nt", "init.lua"), []byte("-- older\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, current := Installed(dir); current {
		t.Error("a changed file counts as current")
	}
	if err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lua", "nt", "extra.lua"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, current := Installed(dir); current {
		t.Error("a file nt does not ship counts as current")
	}
	if err := Install(dir); err != nil { // installing again replaces the folder
		t.Fatal(err)
	}
	if _, current := Installed(dir); !current {
		t.Error("installing again does not make the plugin current")
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), ".nt-tmp-*")); len(left) > 0 {
		t.Errorf("temp folders left: %v", left)
	}

	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if present, _ := Installed(dir); present {
		t.Error("the plugin is still there after Remove")
	}
}
