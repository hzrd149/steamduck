package launcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

// sessionVars are the variables a napplet window needs to reach the screen
// the user is looking at. Kwakore opens windows with the environment its
// service started with, which comes from the systemd user manager.
var sessionVars = []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY"}

const kwakoreUnit = "kwakore.service"

// EnsureSession makes the user manager, and the running kwakore daemon,
// use this process's display. On SteamOS the display differs between game
// mode (gamescope's Xwayland) and desktop mode (Plasma), and the user
// manager keeps whichever one was imported last.
func EnsureSession(ctx context.Context, client *kwakrpc.Client, log *slog.Logger) error {
	want := map[string]string{}
	for _, k := range sessionVars {
		want[k] = os.Getenv(k)
	}
	if want["DISPLAY"] == "" && want["WAYLAND_DISPLAY"] == "" {
		return nil // nothing better to offer; kwakore reports session_unavailable
	}

	mgr, err := managerEnv(ctx)
	if err != nil {
		return err
	}
	var set, unset []string
	for _, k := range sessionVars {
		switch {
		case want[k] != "" && mgr[k] != want[k]:
			set = append(set, k+"="+want[k])
		case want[k] == "" && mgr[k] != "":
			unset = append(unset, k)
		}
	}
	if len(set) > 0 {
		if err := systemctl(ctx, append([]string{"set-environment"}, set...)...); err != nil {
			return err
		}
	}
	if len(unset) > 0 {
		if err := systemctl(ctx, append([]string{"unset-environment"}, unset...)...); err != nil {
			return err
		}
	}

	// A running daemon keeps the environment it started with.
	pid := mainPID(ctx)
	if pid <= 0 {
		return nil // socket activation will start it with the new environment
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil
	}
	have := parseEnviron(raw)
	stale := false
	for _, k := range sessionVars[:2] {
		if have[k] != want[k] {
			stale = true
		}
	}
	if !stale {
		return nil
	}
	st, err := client.Status(ctx)
	if err == nil && st.Health.ActiveWindows > 0 {
		log.Warn("kwakore runs on another display but has open windows; not restarting it",
			"daemon_display", have["DISPLAY"], "display", want["DISPLAY"])
		return nil
	}
	log.Info("restarting kwakore on this display", "display", want["DISPLAY"], "wayland", want["WAYLAND_DISPLAY"])
	_ = client.Close()
	return systemctl(ctx, "restart", kwakoreUnit)
}

func managerEnv(ctx context.Context) (map[string]string, error) {
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "show-environment").Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl --user show-environment: %w", err)
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			env[k] = unquoteSystemd(v)
		}
	}
	return env, nil
}

// unquoteSystemd undoes show-environment's $'...' quoting of unusual values.
func unquoteSystemd(v string) string {
	if strings.HasPrefix(v, "$'") && strings.HasSuffix(v, "'") {
		if s, err := strconv.Unquote(`"` + strings.ReplaceAll(v[2:len(v)-1], `"`, `\"`) + `"`); err == nil {
			return s
		}
	}
	return v
}

func mainPID(ctx context.Context) int {
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "show", "-p", "MainPID", "--value", kwakoreUnit).Output()
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return pid
}

func systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
