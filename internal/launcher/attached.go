package launcher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hzrd149/steamduck/internal/paths"
)

// With kwakore's attached launches, `kwak run ADDRESS` asks the daemon to
// prepare a window and then replaces itself with the window program. The
// window is then a process of whoever ran kwak (Steam), on that caller's
// display. Steam's overlay and gamescope recognize it as the game, Steam
// sees it exit and "Exit game" ends it directly, so none of the legacy
// path's display fixing, window tagging or waiting is needed.

// KwakPath is the CLI of the live kwakore release ($STEAMDUCK_KWAK
// overrides it for development). Its window program is resolved beside
// it, so it must be run from the release, not a copy.
func KwakPath() string {
	if p := os.Getenv("STEAMDUCK_KWAK"); p != "" {
		return p
	}
	return filepath.Join(paths.KwakoreCurrent(), "kwak")
}

var (
	supportOnce sync.Once
	supported   bool
)

// SupportsRun reports whether the installed kwak has `kwak run`. Help
// output never contacts the daemon; an older kwak exits non-zero on it.
func SupportsRun() bool {
	supportOnce.Do(func() {
		if os.Getenv("STEAMDUCK_LEGACY_LAUNCH") != "" {
			return
		}
		supported = exec.Command(KwakPath(), "run", "--help").Run() == nil
	})
	return supported
}

// ExecRun replaces this process with `kwak run ADDRESS`, keeping its PID,
// so the process Steam started becomes the napplet window. When stderr is
// given, the window's diagnostics go there instead of to Steam, which
// discards them.
func ExecRun(address string, stderr *os.File) error {
	if stderr != nil {
		if err := syscall.Dup3(int(stderr.Fd()), 2, 0); err != nil {
			return err
		}
	}
	kwak := KwakPath()
	return syscall.Exec(kwak, []string{kwak, "run", address}, os.Environ())
}

// StartRun starts `kwak run ADDRESS` as a child of this process, for the
// store: the window stays inside the store's process tree, so in game mode
// it belongs to the store's Steam session. A kwak that fails before the
// window starts (daemon down, napplet missing) exits within moments, so
// StartRun waits briefly to report that; the child is reaped in the
// background either way.
func StartRun(address string, stderr *os.File) error {
	cmd := exec.Command(KwakPath(), "run", address)
	var out strings.Builder
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, &limitedWriter{w: &out, n: 4096})
	} else {
		cmd.Stderr = &limitedWriter{w: &out, n: 4096}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			if msg := strings.TrimSpace(out.String()); msg != "" {
				return fmt.Errorf("%s", strings.TrimPrefix(msg, "Error: "))
			}
			return err
		}
	case <-time.After(1500 * time.Millisecond):
	}
	return nil
}

// limitedWriter keeps the first n bytes and discards the rest.
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.n -= k
		_, _ = l.w.Write(p[:k])
	}
	return len(p), nil
}
