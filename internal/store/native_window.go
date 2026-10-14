package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hzrd149/steamduck/internal/flatpak"
	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/launcher"
	"github.com/hzrd149/steamduck/internal/paths"
)

// RunBuiltin keeps the administrative broker on the host and starts only the
// hardened napplet window in the selected host/Flatpak runtime.
func RunBuiltin(ctx context.Context, api *API, cfg paths.Config, debug bool) error {
	dir, err := filepath.EvalSymlinks(paths.KwakoreCurrent())
	if err != nil {
		return fmt.Errorf("find kwakore release: %w", err)
	}
	child := filepath.Join(dir, "kwaklet")
	if _, err := os.Stat(child); err != nil {
		return fmt.Errorf("kwakore window program: %w", err)
	}
	inFlatpak := flatpak.Use(cfg)
	if inFlatpak && !flatpak.Installed(ctx, cfg.FlatpakRuntime) {
		return fmt.Errorf("Flatpak runtime %s is missing; run steamduck runtime install", cfg.FlatpakRuntime)
	}
	command := func(w kwakrpc.AttachedWindow) (*exec.Cmd, error) {
		var cmd *exec.Cmd
		if inFlatpak {
			args := flatpak.WindowArgs(cfg.FlatpakRuntime, dir, w)
			if debug {
				args = append(args[:len(args)-1], "--env=WEBVIEW_DEBUG=true", cfg.FlatpakRuntime)
			}
			cmd = exec.Command("flatpak", args...)
		} else {
			cmd = exec.Command(child)
		}
		cmd.Env = append(os.Environ(), w.Env()...)
		cmd.Env = append(cmd.Env, "WEBVIEW_PATH="+dir)
		if debug {
			cmd.Env = append(cmd.Env, "WEBVIEW_DEBUG=true")
		}
		if os.Getenv("GDK_BACKEND") == "" && os.Getenv("DISPLAY") != "" {
			cmd.Env = append(cmd.Env, "GDK_BACKEND=x11")
		}
		cmd.Stderr = api.Stderr
		if api.AppID != 0 && api.Log != nil {
			go launcher.TagLoop(ctx, w.Instance, api.AppID, api.Log)
		}
		return cmd, nil
	}
	// Games are ordinary attached windows: they get no store extension and their
	// connections live in the child after the parent's copy is closed.
	api.StartWindow = func(callCtx context.Context, address string, trial bool) error {
		w, conn, err := api.Client.LaunchAttached(callCtx, address, trial)
		if err != nil {
			return err
		}
		defer conn.Close()
		file, err := conn.File()
		if err != nil {
			return err
		}
		defer file.Close()
		cmd, err := command(*w)
		if err != nil {
			return err
		}
		cmd.Stdin, cmd.Stdout = file, file
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	if api.Log != nil {
		api.Log.Info("opening trusted store napplet", "runtime", cfg.WindowRuntime, "flatpak", inFlatpak)
	}
	return api.Client.RunBuiltin(ctx, BuiltinDefinition(), command, api.HandleNative)
}
