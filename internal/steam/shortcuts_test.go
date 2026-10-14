package steam

import (
	"path/filepath"
	"testing"

	"github.com/hzrd149/steamduck/internal/steam/vdf"
)

func userShortcut(name string) *vdf.Map {
	e := &vdf.Map{}
	e.Set("appid", int32(-5))
	e.Set("AppName", name)
	e.Set("Exe", `"/usr/bin/`+name+`"`)
	e.Set("LaunchOptions", "")
	return e
}

func TestReconcileKeepsUserShortcuts(t *testing.T) {
	exe := "/home/deck/.local/lib/steamduck/steamduck"
	root := &vdf.Map{}
	list := &vdf.Map{}
	root.Set("shortcuts", list)
	list.Set("0", userShortcut("retroarch"))

	want := []Shortcut{
		NappletShortcut(exe, "35129:aa:snake", "Snake"),
		NappletShortcut(exe, "35129:aa:tetris", "Tetris"),
		{Key: StoreKey, Name: "Steamduck Store", Exe: exe},
	}
	ch := Reconcile(root, want)
	if len(ch.Added) != 3 || len(ch.Removed) != 0 {
		t.Fatalf("change = %+v", ch)
	}
	if len(list.Fields) != 4 || list.Child("0").String("AppName") != "retroarch" {
		t.Fatalf("user shortcut moved or lost: %+v", list.Fields)
	}

	// Steam records playtime; the user hides one shortcut.
	for _, f := range list.Fields {
		e := f.Value.(*vdf.Map)
		if k, _ := KeyOf(e); k == "35129:aa:snake" {
			e.Set("LastPlayTime", int32(1234))
			e.Set("AppName", "renamed by steam?")
		}
	}

	// tetris uninstalled
	ch = Reconcile(root, want[:1])
	if len(ch.Removed) != 2 || len(ch.Updated) != 1 {
		t.Fatalf("second change = %+v", ch)
	}
	if len(list.Fields) != 2 {
		t.Fatalf("fields = %d", len(list.Fields))
	}
	snake := list.Child("1")
	if snake.String("AppName") != "Snake" || snake.Int("LastPlayTime") != 1234 {
		t.Fatalf("snake = %+v", snake)
	}
	if uint32(snake.Int("appid")) != want[0].AppID() || want[0].AppID()&0x80000000 == 0 {
		t.Fatal("appid not stable")
	}

	// nothing to do the third time
	if ch := Reconcile(root, want[:1]); !ch.Empty() {
		t.Fatalf("third change = %+v", ch)
	}
}

func TestSaveLoadShortcuts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "shortcuts.vdf")
	root, err := LoadShortcuts(path)
	if err != nil {
		t.Fatal(err)
	}
	Reconcile(root, []Shortcut{NappletShortcut("/x/steamduck", "35129:aa:snake", "Snake")})
	if err := SaveShortcuts(path, root); err != nil {
		t.Fatal(err)
	}
	again, err := LoadShortcuts(path)
	if err != nil {
		t.Fatal(err)
	}
	if k, ok := KeyOf(again.Child("shortcuts").Child("0")); !ok || k != "35129:aa:snake" {
		t.Fatalf("reloaded key = %q %v", k, ok)
	}
}
