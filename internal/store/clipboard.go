package store

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// copyText puts text on the desktop clipboard with whichever tool the
// session has, for when the webview will not let the page do it.
func copyText(ctx context.Context, text string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var tools [][]string
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		tools = append(tools, []string{"wl-copy"})
	}
	tools = append(tools,
		[]string{"xclip", "-selection", "clipboard"},
		[]string{"xsel", "--clipboard", "--input"},
	)
	for _, t := range tools {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, t[0], t[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if cmd.Run() == nil {
			return nil
		}
	}
	return errors.New("no clipboard is available")
}
