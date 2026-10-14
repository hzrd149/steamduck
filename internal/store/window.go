package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	webview "github.com/abemedia/go-webview"

	"github.com/hzrd149/steamduck/internal/paths"
)

// reply is what every bound call resolves to; errors are values so the UI
// keeps the daemon's error code.
type reply struct {
	Result any            `json:"result,omitempty"`
	Error  map[string]any `json:"error,omitempty"`
}

func (a *API) dispatch(ctx context.Context, method string, params json.RawMessage) reply {
	res, err := a.Call(ctx, method, params)
	if err != nil {
		return reply{Error: ErrorJSON(err)}
	}
	return reply{Result: res}
}

// webviewDir picks the directory holding libwebview.so: $WEBVIEW_PATH, or
// the live kwakore release, whose window program loads the same library.
func webviewDir() (string, error) {
	for _, d := range []string{os.Getenv("WEBVIEW_PATH"), paths.KwakoreCurrent()} {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "libwebview.so")); err == nil {
			return d, nil
		}
	}
	return "", errors.New("libwebview.so not found; install kwakore first (steamduck install)")
}

// RunWindow shows the store until the window closes. It must run on the
// main goroutine, which go-webview locks to the main OS thread.
func RunWindow(ctx context.Context, api *API, debug bool) error {
	dir, err := webviewDir()
	if err != nil {
		return err
	}
	os.Setenv("WEBVIEW_PATH", dir)
	// gamescope composites X11 clients; desktop mode on SteamOS is X11 too.
	if os.Getenv("GDK_BACKEND") == "" && os.Getenv("DISPLAY") != "" {
		os.Setenv("GDK_BACKEND", "x11")
	}

	w := webview.New(debug)
	if w == nil {
		return fmt.Errorf("could not create the store window")
	}
	defer w.Destroy()
	w.SetTitle("Steamduck Store")
	w.SetSize(1280, 800, webview.HintNone)

	// Closing the window from another goroutine (a signal, the UI's quit)
	// must not touch the webview once Run has returned: it is destroyed
	// then, and a late Dispatch crashes the process.
	var mu sync.Mutex
	running := true
	terminate := func() {
		mu.Lock()
		defer mu.Unlock()
		if running {
			w.Dispatch(w.Terminate)
		}
	}
	api.Quit = terminate

	if err := w.Bind("steamduckCall", func(method string, params json.RawMessage) reply {
		return api.dispatch(ctx, method, params)
	}); err != nil {
		return err
	}
	page, err := Page()
	if err != nil {
		return err
	}
	w.SetHtml(page)
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminate()
		case <-finished:
		}
	}()
	w.Run()
	mu.Lock()
	running = false
	mu.Unlock()
	close(finished)
	return nil
}
