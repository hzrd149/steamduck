package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowPIDs(t *testing.T) {
	root := t.TempDir()
	write := func(pid, env string) {
		_ = os.MkdirAll(filepath.Join(root, pid), 0o755)
		_ = os.WriteFile(filepath.Join(root, pid, "environ"), []byte(env), 0o644)
	}
	write("100", "PATH=/bin\x00KWAKORE_INSTANCE_ID=abc\x00")
	write("101", "KWAKORE_INSTANCE_ID=abc\x00HOME=/h\x00") // WebKit helper
	write("102", "KWAKORE_INSTANCE_ID=abcd\x00")           // another window
	write("self", "KWAKORE_INSTANCE_ID=abc\x00")
	got := windowPIDs(root, "abc")
	if len(got) != 2 || !got[100] || !got[101] {
		t.Fatalf("pids = %v", got)
	}
}

func TestAppIDFromEnv(t *testing.T) {
	t.Setenv("SteamAppId", "0")
	t.Setenv("SteamGameId", "")
	if AppIDFromEnv() != 0 {
		t.Fatal("expected 0")
	}
	t.Setenv("SteamGameId", "3141592653")
	if AppIDFromEnv() != 3141592653 {
		t.Fatal("32-bit id")
	}
	t.Setenv("SteamGameId", "13493037702022430720") // 3141592653<<32 | 0x02000000
	if got := AppIDFromEnv(); got != 3141592653 {
		t.Fatalf("64-bit id gave %d", got)
	}
}

func TestUnquoteSystemd(t *testing.T) {
	if unquoteSystemd(`$'a\nb'`) != "a\nb" || unquoteSystemd(":0") != ":0" {
		t.Fatal("unquote")
	}
}
