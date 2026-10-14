package installer

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestRemovePathLines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	// what install.sh appends, between the user's own lines
	rc := "alias ll='ls -l'\n\n" + pathMarker + "\ncase \":$PATH:\" in *) ;; esac\nexport EDITOR=vim\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0o644); err != nil {
		t.Fatal(err)
	}
	fish := filepath.Join(home, ".config", "fish", "conf.d", "steamduck.fish")
	if err := os.MkdirAll(filepath.Dir(fish), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fish, []byte(pathMarker+"\nset -gx PATH x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	removePathLines(slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if want := "alias ll='ls -l'\nexport EDITOR=vim\n"; string(got) != want {
		t.Errorf(".bashrc = %q, want %q", got, want)
	}
	if _, err := os.Stat(fish); !os.IsNotExist(err) {
		t.Errorf("fish file still there: %v", err)
	}
}
