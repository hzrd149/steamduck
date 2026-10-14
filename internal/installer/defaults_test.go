package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKwakoreDefaults(t *testing.T) {
	for _, initial := range []string{"", `{"relays":[],"theme":"dark"}`, `{"gnome_search":true}`, `{"gnome_search":false}`} {
		t.Run(initial, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", root)
			t.Setenv("KWAKORE_CONFIG_FILE", "")
			path := filepath.Join(root, "kwakore", "config.json")
			if initial != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := configureKwakoreDefaults(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]any
			if err := json.Unmarshal(data, &actual); err != nil {
				t.Fatal(err)
			}
			if actual["gnome_search"] != (initial == `{"gnome_search":true}`) {
				t.Fatalf("unexpected defaults: %s", data)
			}
			if initial == `{"relays":[],"theme":"dark"}` && (actual["theme"] != "dark" || len(actual["relays"].([]any)) != 0) {
				t.Fatalf("lost configuration: %s", data)
			}
			if err := configureKwakoreDefaults(); err != nil {
				t.Fatal(err)
			}
			again, _ := os.ReadFile(path)
			if string(again) != string(data) {
				t.Fatal("default setup was not idempotent")
			}
		})
	}
}

func TestKwakoreDefaultsRejectInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed.json")
	t.Setenv("KWAKORE_CONFIG_FILE", path)
	for _, invalid := range []string{"null", "[]", "broken"} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := configureKwakoreDefaults(); err == nil {
			t.Fatal("accepted invalid configuration")
		}
		data, _ := os.ReadFile(path)
		if string(data) != invalid {
			t.Fatal("modified invalid configuration")
		}
	}
}
