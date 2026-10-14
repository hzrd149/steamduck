package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Steamduck does not need GNOME search. Set a declarative default before the
// daemon starts; existing configuration and persisted overrides take priority.
func configureKwakoreDefaults() error {
	path := os.Getenv("KWAKORE_CONFIG_FILE")
	if path == "" {
		path = filepath.Join(xdgConfigHome(), "kwakore", "config.json")
	}
	if !filepath.IsAbs(path) {
		return errors.New("KWAKORE_CONFIG_FILE must be absolute")
	}
	config := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &config); err != nil || config == nil {
			return fmt.Errorf("invalid kwakore configuration in %s", path)
		}
	}
	if _, explicit := config["gnome_search"]; explicit {
		return nil
	}
	config["gnome_search"] = json.RawMessage("false")
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Write in place to preserve a user's configuration symlink.
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
