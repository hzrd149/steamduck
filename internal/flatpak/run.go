package flatpak

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

// ExecWindow opens address as an attached window and replaces this process
// with `flatpak run` of kwakore's window program inside runtime. The PID
// Steam started stays the window's, as with `kwak run`. Window diagnostics
// go to stderr when it is given.
func ExecWindow(ctx context.Context, client *kwakrpc.Client, address string, try bool, runtime string, stderr *os.File) error {
	flatpakBin, err := exec.LookPath("flatpak")
	if err != nil {
		return fmt.Errorf("flatpak is not installed: %w", err)
	}
	if !Installed(ctx, runtime) {
		return fmt.Errorf("the Flatpak runtime %s is not installed; run `steamduck runtime install`", runtime)
	}
	releaseDir, err := ReleaseDir()
	if err != nil {
		return err
	}
	w, conn, err := client.LaunchAttached(ctx, address, try)
	if err != nil {
		return err
	}
	if err := connToStdio(conn, stderr); err != nil {
		return err
	}
	return syscall.Exec(flatpakBin, append([]string{flatpakBin}, WindowArgs(runtime, releaseDir, *w)...), os.Environ())
}

// connToStdio makes conn the process's stdin and stdout, and stderr its
// fd 2, for a program about to be exec'd.
func connToStdio(conn interface{ File() (*os.File, error) }, stderr *os.File) error {
	f, err := conn.File()
	if err != nil {
		return err
	}
	defer f.Close()
	for _, fd := range []int{0, 1} {
		if err := syscall.Dup3(int(f.Fd()), fd, 0); err != nil {
			return err
		}
	}
	if stderr != nil {
		if err := syscall.Dup3(int(stderr.Fd()), 2, 0); err != nil {
			return err
		}
	}
	return nil
}

// StartWindow opens address as an attached window and starts kwakore's
// window program as a child of this process, which must already run inside
// the runtime (the store's sandbox). The child is reaped in the background.
func StartWindow(ctx context.Context, client *kwakrpc.Client, address string, try bool, stderr *os.File) error {
	releaseDir, err := ReleaseDir()
	if err != nil {
		return err
	}
	w, conn, err := client.LaunchAttached(ctx, address, try)
	if err != nil {
		return err
	}
	defer conn.Close()
	f, err := conn.File()
	if err != nil {
		return err
	}
	defer f.Close()
	w.Name = titleText(w.Name)
	cmd := exec.Command(filepath.Join(releaseDir, "kwaklet"))
	cmd.Stdin, cmd.Stdout = f, f
	if stderr != nil {
		cmd.Stderr = stderr
	}
	cmd.Env = append(os.Environ(), w.Env()...)
	cmd.Env = append(cmd.Env, "WEBVIEW_PATH="+releaseDir)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
